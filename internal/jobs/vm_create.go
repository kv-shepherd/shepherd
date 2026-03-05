package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	"kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
)

// ---------------------------------------------------------------------------
// Job Args
// ---------------------------------------------------------------------------

// VMCreateArgs carries only EventID (Claim-check pattern, ADR-0009).
type VMCreateArgs struct {
	EventID string `json:"event_id"`
}

// Kind returns the job kind identifier for VM creation.
func (VMCreateArgs) Kind() string { return "vm_create" }

// InsertOpts returns default insert options for VM creation jobs.
func (VMCreateArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "vm_operations",
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
		},
	}
}

// ---------------------------------------------------------------------------
// Worker
// ---------------------------------------------------------------------------

// VMCreateWorker processes VM creation jobs after approval.
//
// Execution flow (master-flow.md Stage 5.C):
//  1. Fetch DomainEvent by EventID (claim-check, ADR-0009)
//  2. Parse VMCreationPayload
//  3. Fetch ApprovalTicket for admin-determined fields (cluster, storage)
//  4. Build effective spec from payload + ticket modifications
//  5. Idempotency check: detect duplicate VM by event label
//  6. Execute K8s VM creation via VMService (outside transaction, ADR-0012)
//  7. Update event status to COMPLETED or FAILED
type VMCreateWorker struct {
	river.WorkerDefaults[VMCreateArgs]
	entClient   *ent.Client
	vmService   *service.VMService
	auditLogger *audit.Logger
}

// NewVMCreateWorker creates a new VMCreateWorker with all dependencies (ADR-0013 manual DI).
func NewVMCreateWorker(entClient *ent.Client, vmService *service.VMService, auditLogger *audit.Logger) *VMCreateWorker {
	return &VMCreateWorker{entClient: entClient, vmService: vmService, auditLogger: auditLogger}
}

// findCreatedVMByEvent performs an idempotency check by searching for an
// existing VM that was already created by a prior attempt with the same eventID.
func (w *VMCreateWorker) findCreatedVMByEvent(
	ctx context.Context,
	clusterID, namespace, eventID string,
) (*domain.VM, error) {
	list, err := w.vmService.ListVMs(ctx, clusterID, namespace, provider.ListOptions{
		LabelSelector: "shepherd.io/event-id=" + eventID,
		Limit:         1,
	})
	if err != nil {
		return nil, err
	}
	for _, candidate := range list.Items {
		if candidate == nil {
			continue
		}
		if candidate.Spec.Labels["shepherd.io/event-id"] == eventID {
			return candidate, nil
		}
	}
	return nil, nil
}

// Work executes the VM creation.
func (w *VMCreateWorker) Work(ctx context.Context, job *river.Job[VMCreateArgs]) error {
	eventID := job.Args.EventID

	logger.Info("Processing VM creation job",
		zap.String("event_id", eventID),
		zap.Int64("attempt", int64(job.Attempt)),
	)

	// Step 1: Fetch DomainEvent (claim-check pattern).
	event, err := w.entClient.DomainEvent.Get(ctx, eventID)
	if err != nil {
		return fmt.Errorf("fetch domain event %s: %w", eventID, err)
	}
	if event.Status == domainevent.StatusCOMPLETED {
		logger.Info("vm create event already completed, skipping duplicate execution",
			zap.String("event_id", eventID),
		)
		return nil
	}
	setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusEXECUTING)

	// Step 2: Parse payload.
	var payload domain.VMCreationPayload
	if decodeErr := json.Unmarshal(event.Payload, &payload); decodeErr != nil {
		_, _ = w.entClient.DomainEvent.UpdateOneID(eventID).SetStatus(domainevent.StatusFAILED).Save(ctx)
		setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusFAILED)
		return river.JobCancel(fmt.Errorf("unmarshal payload for event %s: %w", eventID, decodeErr))
	}
	markFailed := func(cause error, cancel bool) error {
		if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
			SetStatus(domainevent.StatusFAILED).
			Save(ctx); saveErr != nil {
			logger.Error("failed to persist FAILED status for event",
				zap.String("event_id", eventID),
				zap.Error(saveErr),
			)
		}
		setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusFAILED)
		if cancel {
			return river.JobCancel(cause)
		}
		return cause
	}
	namespace := strings.TrimSpace(payload.Namespace)
	if namespace == "" {
		return markFailed(fmt.Errorf("event %s payload namespace is empty", eventID), true)
	}

	// Step 3: Fetch approval ticket for admin-determined fields.
	ticket, err := w.entClient.ApprovalTicket.Query().
		Where(approvalticket.EventIDEQ(eventID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return markFailed(fmt.Errorf("approval ticket missing for event %s", eventID), true)
		}
		return fmt.Errorf("query approval ticket for event %s: %w", eventID, err)
	}
	clusterID := strings.TrimSpace(ticket.SelectedClusterID)
	if clusterID == "" {
		return markFailed(fmt.Errorf("event %s has no selected cluster", eventID), true)
	}
	if err := w.ensureNamespaceClusterEnvironment(ctx, clusterID, namespace); err != nil {
		return markFailed(
			fmt.Errorf("event %s namespace/cluster environment validation failed: %w", eventID, err),
			true,
		)
	}

	// Step 4: Build effective spec.
	effectiveTemplateID, effectiveInstanceSizeID := resolveEffectiveSelectionIDs(payload, ticket.ModifiedSpec)
	if effectiveTemplateID == "" {
		return markFailed(fmt.Errorf("event %s has empty effective template id", eventID), true)
	}
	if effectiveInstanceSizeID == "" {
		return markFailed(fmt.Errorf("event %s has empty effective instance size id", eventID), true)
	}

	vmRow, err := w.entClient.VM.Query().
		Where(vm.TicketIDEQ(ticket.ID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return markFailed(fmt.Errorf("vm row missing for ticket %s", ticket.ID), true)
		}
		return fmt.Errorf("query vm row for ticket %s: %w", ticket.ID, err)
	}
	vmName := strings.TrimSpace(vmRow.Name)
	if vmName == "" {
		return markFailed(fmt.Errorf("vm row for ticket %s has empty name", ticket.ID), true)
	}

	size, err := w.entClient.InstanceSize.Get(ctx, effectiveInstanceSizeID)
	if err != nil {
		if ent.IsNotFound(err) {
			return markFailed(fmt.Errorf("instance size %s not found", effectiveInstanceSizeID), true)
		}
		return fmt.Errorf("query instance size %s: %w", effectiveInstanceSizeID, err)
	}
	cpu := size.CPUCores
	memoryGi := size.MemoryGi
	diskGB := size.DiskGB
	applyInstanceSizeSnapshotOverrides(&cpu, &memoryGi, &diskGB, ticket.InstanceSizeSnapshot)
	specOverrides := resolveInstanceSizeSpecOverrides(size.SpecOverrides, ticket.InstanceSizeSnapshot)

	tpl, err := w.entClient.Template.Get(ctx, effectiveTemplateID)
	if err != nil {
		if ent.IsNotFound(err) {
			return markFailed(fmt.Errorf("template %s not found", effectiveTemplateID), true)
		}
		return fmt.Errorf("query template %s: %w", effectiveTemplateID, err)
	}
	// ADR-0036: Template.spec replaced by semantic fields (source_type, image_url, pvc_name).
	// Use snapshot override if available (populated at approval time), otherwise live values.
	var image string
	if len(ticket.TemplateSnapshot) > 0 {
		image, err = extractTemplateImageFromSnapshot(ticket.TemplateSnapshot)
	} else {
		image, err = extractTemplateImageFromEnt(tpl)
	}
	if err != nil {
		return markFailed(fmt.Errorf("resolve image from template %s: %w", effectiveTemplateID, err), true)
	}
	cloudInit := tpl.CloudInit
	if len(ticket.TemplateSnapshot) > 0 {
		if snapCloudInit, ok := extractTemplateCloudInitFromSnapshot(ticket.TemplateSnapshot); ok {
			cloudInit = snapCloudInit
		}
	}

	spec := &domain.VMSpec{
		Name:      vmName,
		CPU:       cpu,
		MemoryGi:  memoryGi,
		DiskGB:    diskGB,
		Image:     image,
		CloudInit: cloudInit,
		Labels: map[string]string{
			"shepherd.io/service-id":  payload.ServiceID,
			"shepherd.io/template-id": effectiveTemplateID,
			"shepherd.io/event-id":    eventID,
		},
		SpecOverrides: specOverrides,
	}
	applyModifiedSpecOverrides(spec, ticket.ModifiedSpec)
	if spec.CPU <= 0 || spec.MemoryGi <= 0 || strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Image) == "" {
		return markFailed(fmt.Errorf(
			"invalid effective vm spec for event %s (name=%q cpu=%.1f memory_gi=%.1f image=%q)",
			eventID, spec.Name, spec.CPU, spec.MemoryGi, spec.Image,
		), true)
	}

	// Step 5: Idempotency check.
	// If a prior attempt already created this VM, detect it by event label and skip create.
	createdVM, err := w.findCreatedVMByEvent(ctx, clusterID, namespace, eventID)
	if err != nil {
		return fmt.Errorf("check vm create idempotency for event %s: %w", eventID, err)
	}

	var createdVMName string
	var targetVMStatus vm.Status
	if createdVM != nil {
		createdVMName = createdVM.Name
		targetVMStatus = mapCreatedVMStatusToRow(createdVM)
		logger.Info("found existing VM for event, skipping duplicate create",
			zap.String("event_id", eventID),
			zap.String("vm_name", createdVMName),
		)
	} else {
		// Step 6: Execute K8s VM creation (outside transaction per ADR-0012).
		vmObj, err := w.vmService.ExecuteK8sCreate(ctx, clusterID, namespace, spec)
		if err != nil {
			// K8s VM was NOT created — safe to retry.
			// Persist FAILED status (best-effort; original error is returned regardless).
			if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
				SetStatus(domainevent.StatusFAILED).
				Save(ctx); saveErr != nil {
				logger.Error("failed to persist FAILED status for event",
					zap.String("event_id", eventID),
					zap.Error(saveErr),
				)
			}
			if _, saveErr := w.entClient.VM.UpdateOneID(vmRow.ID).
				SetStatus(vm.StatusFAILED).
				Save(ctx); saveErr != nil {
				logger.Error("failed to persist VM FAILED status",
					zap.String("event_id", eventID),
					zap.String("vm_id", vmRow.ID),
					zap.Error(saveErr),
				)
			}

			logAuditVMOp(ctx, w.auditLogger, "create_failed", eventID, "system", eventID)
			setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusFAILED)

			return fmt.Errorf("execute k8s create for event %s: %w", eventID, err)
		}
		createdVMName = vmObj.Name
		targetVMStatus = mapCreatedVMStatusToRow(vmObj)
	}

	// Step 7: Update VM status in DB.
	if _, saveErr := w.entClient.VM.UpdateOneID(vmRow.ID).
		SetStatus(targetVMStatus).
		Save(ctx); saveErr != nil {
		return fmt.Errorf(
			"persist vm status for event %s (vm_id=%s, status=%s): %w",
			eventID, vmRow.ID, targetVMStatus, saveErr,
		)
	}

	// Step 8: Update event status to COMPLETED.
	// We deliberately return error on persistence failure so River retries.
	// Retry is safe because idempotency check above prevents duplicate K8s create.
	if _, saveErr := w.entClient.DomainEvent.UpdateOneID(eventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(ctx); saveErr != nil {
		return fmt.Errorf("persist COMPLETED status for event %s: %w", eventID, saveErr)
	}
	setTicketStatusByEvent(ctx, w.entClient, eventID, approvalticket.StatusSUCCESS)

	logAuditVMOp(ctx, w.auditLogger, "create", createdVMName, "system", eventID)

	logger.Info("VM creation job completed",
		zap.String("event_id", eventID),
		zap.String("vm_name", createdVMName),
		zap.String("cluster", clusterID),
	)

	return nil
}

func (w *VMCreateWorker) ensureNamespaceClusterEnvironment(
	ctx context.Context,
	clusterID string,
	namespace string,
) error {
	cl, err := w.entClient.Cluster.Get(ctx, strings.TrimSpace(clusterID))
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("cluster %s not found", clusterID)
		}
		return fmt.Errorf("query cluster %s: %w", clusterID, err)
	}
	if cl.Status != cluster.StatusHEALTHY {
		return fmt.Errorf("cluster %s is not healthy (status: %s)", cl.ID, cl.Status)
	}

	nsName := strings.TrimSpace(namespace)
	ns, err := w.entClient.NamespaceRegistry.Query().
		Where(namespaceregistry.NameEQ(nsName)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("namespace %s not found in registry", nsName)
		}
		return fmt.Errorf("query namespace %s: %w", nsName, err)
	}
	if !ns.Enabled {
		return fmt.Errorf("namespace %s is disabled", ns.Name)
	}

	return validateNamespaceClusterEnvironment(string(ns.Environment), string(cl.Environment))
}

func validateNamespaceClusterEnvironment(namespaceEnv, clusterEnv string) error {
	nsEnv := strings.TrimSpace(strings.ToLower(namespaceEnv))
	clEnv := strings.TrimSpace(strings.ToLower(clusterEnv))
	if nsEnv == "" || clEnv == "" {
		return fmt.Errorf("namespace/cluster environment is incomplete (namespace=%q cluster=%q)", namespaceEnv, clusterEnv)
	}
	if nsEnv != clEnv {
		return fmt.Errorf(
			"namespace environment %q does not match cluster environment %q",
			nsEnv,
			clEnv,
		)
	}
	return nil
}

func mapCreatedVMStatusToRow(created *domain.VM) vm.Status {
	if created == nil {
		return vm.StatusRUNNING
	}

	// Stage 5.C in master-flow mandates CREATING -> RUNNING|FAILED.
	// For create-time provider statuses other than explicit FAILED, we persist RUNNING
	// and rely on vm_status_sync (ADR-0038) for later reconciliation.
	switch created.Status {
	case domain.VMStatusFailed:
		return vm.StatusFAILED
	default:
		return vm.StatusRUNNING
	}
}

func resolveEffectiveSelectionIDs(
	payload domain.VMCreationPayload,
	modifiedSpec map[string]interface{},
) (templateID, instanceSizeID string) {
	templateID = strings.TrimSpace(payload.TemplateID)
	instanceSizeID = strings.TrimSpace(payload.InstanceSizeID)
	if override := lookupStringValue(modifiedSpec, "template_id"); override != "" {
		templateID = override
	}
	if override := lookupStringValue(modifiedSpec, "instance_size_id"); override != "" {
		instanceSizeID = override
	}
	return templateID, instanceSizeID
}

func applyInstanceSizeSnapshotOverrides(cpu, memoryGi *float64, diskGB *int, snapshot map[string]interface{}) {
	if cpu == nil || memoryGi == nil || diskGB == nil {
		return
	}
	if v, ok := lookupFloat64Value(snapshot, "cpu_cores", "cpu"); ok {
		*cpu = v
	}
	if v, ok := lookupFloat64Value(snapshot, "memory_gi"); ok {
		*memoryGi = v
	}
	if v, ok := lookupIntValue(snapshot, "disk_gb", "disk"); ok && v >= 0 {
		*diskGB = v
	}
}

func resolveInstanceSizeSpecOverrides(
	baseOverrides map[string]interface{},
	snapshot map[string]interface{},
) map[string]interface{} {
	if snapOverrides := extractSpecOverridesFromSnapshot(snapshot); len(snapOverrides) > 0 {
		return snapOverrides
	}
	return cloneMapValues(baseOverrides)
}

func extractSpecOverridesFromSnapshot(snapshot map[string]interface{}) map[string]interface{} {
	if len(snapshot) == 0 {
		return nil
	}
	if raw, ok := lookupValue(snapshot, "spec_overrides"); ok {
		if overrides, ok := toMap(raw); ok {
			return cloneMapValues(overrides)
		}
	}
	// Backward compatibility: some rows may already store path->value pairs directly.
	if isLikelySpecOverrideMap(snapshot) {
		return cloneMapValues(snapshot)
	}
	return nil
}

func applyModifiedSpecOverrides(spec *domain.VMSpec, modifiedSpec map[string]interface{}) {
	if spec == nil || len(modifiedSpec) == 0 {
		return
	}
	// cpu_limit takes precedence over cpu for the CPU limit value.
	if v, ok := lookupFloat64Value(modifiedSpec, "cpu_limit"); ok && v > 0 {
		spec.CPU = v
	} else if v, ok := lookupFloat64Value(modifiedSpec, "cpu", "resources.cpu"); ok {
		spec.CPU = v
	}
	// cpu_request maps to spec.CPURequest for K8s overcommit.
	if v, ok := lookupFloat64Value(modifiedSpec, "cpu_request"); ok && v > 0 {
		spec.CPURequest = v
	}
	// memory_limit_gi takes precedence over memory_gi for the memory limit value.
	if v, ok := lookupFloat64Value(modifiedSpec, "memory_limit_gi"); ok && v > 0 {
		spec.MemoryGi = v
	} else if v, ok := lookupFloat64Value(modifiedSpec, "memory_gi", "resources.memory_gi"); ok {
		spec.MemoryGi = v
	}
	// memory_request_gi maps to spec.MemoryRequestGi for K8s overcommit.
	if v, ok := lookupFloat64Value(modifiedSpec, "memory_request_gi"); ok && v > 0 {
		spec.MemoryRequestGi = v
	}
	if v, ok := lookupIntValue(modifiedSpec, "disk_gb", "resources.disk_gb"); ok && v >= 0 {
		spec.DiskGB = v
	}
	if image, err := extractTemplateImage(modifiedSpec); err == nil && strings.TrimSpace(image) != "" {
		spec.Image = image
	}
	spec.SpecOverrides = applySpecOverridePatches(spec.SpecOverrides, extractSpecOverridesFromModifiedSpec(modifiedSpec))
}

func extractSpecOverridesFromModifiedSpec(modifiedSpec map[string]interface{}) map[string]interface{} {
	if len(modifiedSpec) == 0 {
		return nil
	}
	overrides := map[string]interface{}{}
	if raw, ok := lookupValue(modifiedSpec, "spec_overrides"); ok {
		if nested, ok := toMap(raw); ok {
			for k, v := range nested {
				key := strings.TrimSpace(k)
				if key == "" {
					continue
				}
				overrides[key] = v
			}
		}
	}
	for k, v := range modifiedSpec {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if key == "spec" || strings.HasPrefix(key, "spec.") {
			overrides[key] = v
		}
	}
	if len(overrides) == 0 {
		return nil
	}
	return overrides
}

func applySpecOverridePatches(
	base map[string]interface{},
	patches map[string]interface{},
) map[string]interface{} {
	if len(base) == 0 && len(patches) == 0 {
		return nil
	}
	merged := cloneMapValues(base)
	for k, v := range patches {
		merged[k] = v
	}
	return merged
}

func cloneMapValues(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func isLikelySpecOverrideMap(values map[string]interface{}) bool {
	for key := range values {
		trimmed := strings.TrimSpace(key)
		if trimmed == "spec" || strings.HasPrefix(trimmed, "spec.") {
			return true
		}
	}
	return false
}

func extractTemplateImage(templateSpec map[string]interface{}) (string, error) {
	if image := lookupStringValue(templateSpec, "image", "image_source.image", "source.image"); image != "" {
		return image, nil
	}
	if pvc := lookupStringValue(
		templateSpec,
		"pvc_name",
		"image_source.pvc_name",
		"image_source.pvc.name",
		"source.pvc_name",
		"source.pvc.name",
	); pvc != "" {
		return "pvc:" + pvc, nil
	}

	for _, path := range []string{
		"spec.template.spec.volumes",
		"template.spec.volumes",
		"volumes",
	} {
		raw, ok := lookupValue(templateSpec, path)
		if !ok {
			continue
		}
		if image := extractImageFromVolumes(raw); image != "" {
			return image, nil
		}
	}

	return "", fmt.Errorf("no supported image source found in template spec")
}

// extractTemplateImageFromEnt resolves the boot image from an Ent Template object.
// ADR-0036: Template no longer stores spec JSONB; image_url and pvc_name are explicit fields.
// PVC image format: "pvc:namespace/name" when pvc_namespace is set, "pvc:name" otherwise (legacy).
func extractTemplateImageFromEnt(tpl *ent.Template) (string, error) {
	if tpl.ImageURL != "" {
		return tpl.ImageURL, nil
	}
	if tpl.PvcName != "" {
		if tpl.PvcNamespace != "" {
			return "pvc:" + tpl.PvcNamespace + "/" + tpl.PvcName, nil
		}
		return "pvc:" + tpl.PvcName, nil
	}
	return "", fmt.Errorf("template %s has no image_url or pvc_name configured", tpl.ID)
}

// extractTemplateImageFromSnapshot resolves the boot image from a template snapshot map
// stored at approval time. Snapshot keys use the ADR-0036 semantic format:
// source_type, image_url, pvc_name, pvc_namespace. Falls back to legacy spec-map lookup.
func extractTemplateImageFromSnapshot(snapshot map[string]interface{}) (string, error) {
	// ADR-0036 format (new snapshots)
	sourceType := lookupStringValue(snapshot, "source_type")
	switch sourceType {
	case "image":
		if url := lookupStringValue(snapshot, "image_url"); url != "" {
			return url, nil
		}
	case "pvc":
		if pvc := lookupStringValue(snapshot, "pvc_name"); pvc != "" {
			if ns := lookupStringValue(snapshot, "pvc_namespace"); ns != "" {
				return "pvc:" + ns + "/" + pvc, nil
			}
			return "pvc:" + pvc, nil
		}
	}

	// Legacy format fallback (old snapshots stored via spec JSONB before ADR-0036)
	return extractTemplateImage(snapshot)
}

// extractTemplateCloudInitFromSnapshot resolves cloud-init userdata from a
// template snapshot map. Returns (value, true) when the key is present; this
// lets callers distinguish "missing key" from "explicitly empty cloud_init".
func extractTemplateCloudInitFromSnapshot(snapshot map[string]interface{}) (string, bool) {
	if len(snapshot) == 0 {
		return "", false
	}
	raw, ok := lookupValue(snapshot, "cloud_init")
	if !ok {
		return "", false
	}
	return toString(raw), true
}

func extractImageFromVolumes(raw interface{}) string {
	items, ok := raw.([]interface{})
	if !ok {
		return ""
	}
	for _, item := range items {
		volume, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if containerDisk, ok := volume["containerDisk"].(map[string]interface{}); ok {
			if image := strings.TrimSpace(toString(containerDisk["image"])); image != "" {
				return image
			}
		}
		if pvc, ok := volume["persistentVolumeClaim"].(map[string]interface{}); ok {
			if claimName := strings.TrimSpace(toString(pvc["claimName"])); claimName != "" {
				return "pvc:" + claimName
			}
		}
	}
	return ""
}

func lookupStringValue(values map[string]interface{}, paths ...string) string {
	for _, path := range paths {
		raw, ok := lookupValue(values, path)
		if !ok {
			continue
		}
		if str := strings.TrimSpace(toString(raw)); str != "" {
			return str
		}
	}
	return ""
}

func lookupIntValue(values map[string]interface{}, paths ...string) (int, bool) {
	for _, path := range paths {
		raw, ok := lookupValue(values, path)
		if !ok {
			continue
		}
		if v, ok := toInt(raw); ok {
			return v, true
		}
	}
	return 0, false
}

func lookupFloat64Value(values map[string]interface{}, paths ...string) (float64, bool) {
	for _, path := range paths {
		raw, ok := lookupValue(values, path)
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case float64:
			return v, true
		case float32:
			return float64(v), true
		case int:
			return float64(v), true
		case int64:
			return float64(v), true
		case int32:
			return float64(v), true
		}
	}
	return 0, false
}

func lookupValue(values map[string]interface{}, path string) (interface{}, bool) {
	if len(values) == 0 || path == "" {
		return nil, false
	}
	if v, ok := values[path]; ok {
		return v, true
	}
	current := interface{}(values)
	for _, segment := range strings.Split(path, ".") {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		next, ok := m[segment]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func toMap(raw interface{}) (map[string]interface{}, bool) {
	switch v := raw.(type) {
	case map[string]interface{}:
		return v, true
	default:
		return nil, false
	}
}

func toInt(raw interface{}) (int, bool) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1

	switch v := raw.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		if v > int64(maxInt) || v < int64(minInt) {
			return 0, false
		}
		return int(v), true
	case uint:
		if uint64(v) > uint64(maxInt) {
			return 0, false
		}
		return int(v), true // #nosec G115 -- guarded by maxInt bound check above.
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		if uint64(v) > uint64(maxInt) {
			return 0, false
		}
		return int(v), true // #nosec G115 -- guarded by maxInt bound check above.
	case uint64:
		if v > uint64(maxInt) {
			return 0, false
		}
		return int(v), true // #nosec G115 -- guarded by maxInt bound check above.
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return i, true
		}
	}
	return 0, false
}

func toString(raw interface{}) string {
	if raw == nil {
		return ""
	}
	if v, ok := raw.(string); ok {
		return v
	}
	return fmt.Sprint(raw)
}
