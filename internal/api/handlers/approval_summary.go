package handlers

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

type approvalServiceLookup struct {
	ServiceID   string
	ServiceName string
	SystemID    string
	SystemName  string
}

type approvalVMContext struct {
	VMID               string
	VMName             string
	LatestVMStatus     string
	Namespace          string
	ClusterID          string
	ClusterName        string
	ClusterEnvironment string
	ServiceID          string
	ServiceName        string
	SystemID           string
	SystemName         string
	TemplateID         string
	InstanceSizeID     string
	CurrentCPUCores    float64
	CurrentMemoryGi    float64
	CurrentDiskGB      int
}

func buildApprovalSystemIDByServiceID(
	serviceByID map[string]approvalServiceLookup,
) map[string]string {
	byServiceID := make(map[string]string, len(serviceByID))
	for serviceID, lookup := range serviceByID {
		if strings.TrimSpace(serviceID) == "" || strings.TrimSpace(lookup.SystemID) == "" {
			continue
		}
		byServiceID[serviceID] = lookup.SystemID
	}
	return byServiceID
}

func collectApprovalSummaryVMIDs(eventPayloadMap map[string][]byte) []string {
	vmIDSet := make(map[string]struct{})
	for _, raw := range eventPayloadMap {
		if len(raw) == 0 {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		collectVMIDsFromApprovalPayload(payload, vmIDSet)
	}
	return sortedStringSet(vmIDSet)
}

func collectVMIDsFromApprovalPayload(
	payload map[string]interface{},
	vmIDs map[string]struct{},
) {
	if len(payload) == 0 {
		return
	}
	if vmID := trimPayloadString(payload["vm_id"]); vmID != "" {
		vmIDs[vmID] = struct{}{}
	}
	items, ok := payload["items"].([]interface{})
	if !ok {
		return
	}
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if vmID := trimPayloadString(itemMap["vm_id"]); vmID != "" {
			vmIDs[vmID] = struct{}{}
		}
	}
}

func (s *Server) loadApprovalServiceLookups(
	ctx context.Context,
	serviceIDs []string,
) map[string]approvalServiceLookup {
	byServiceID := make(map[string]approvalServiceLookup, len(serviceIDs))
	if len(serviceIDs) == 0 {
		return byServiceID
	}

	services, err := s.client.Service.Query().
		Where(entservice.IDIn(serviceIDs...)).
		WithSystem().
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch services for approval summary", zap.Error(err))
		return byServiceID
	}

	for _, svc := range services {
		if svc == nil {
			continue
		}
		lookup := approvalServiceLookup{
			ServiceID:   svc.ID,
			ServiceName: svc.Name,
		}
		if svc.Edges.System != nil {
			lookup.SystemID = svc.Edges.System.ID
			lookup.SystemName = svc.Edges.System.Name
		}
		byServiceID[svc.ID] = lookup
	}
	return byServiceID
}

func (s *Server) loadApprovalVMContexts(
	ctx context.Context,
	vmIDs []string,
) (byVMID map[string]approvalVMContext, extraTemplateIDs, extraInstanceSizeIDs []string) {
	byVMID = make(map[string]approvalVMContext, len(vmIDs))
	if len(vmIDs) == 0 {
		return byVMID, nil, nil
	}

	vms, err := s.client.VM.Query().
		Where(entvm.IDIn(vmIDs...)).
		WithService(func(query *ent.ServiceQuery) {
			query.WithSystem()
		}).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch VMs for approval summary", zap.Error(err))
		return byVMID, nil, nil
	}

	vms = s.refreshVMLiveStates(ctx, vms)

	clusterIDSet := make(map[string]struct{})
	createTicketIDToVMID := make(map[string]string)
	for _, vmRow := range vms {
		if vmRow == nil {
			continue
		}
		ctxRow := approvalVMContext{
			VMID:           vmRow.ID,
			VMName:         vmRow.Name,
			LatestVMStatus: string(vmRow.Status),
			Namespace:      vmRow.Namespace,
			ClusterID:      vmRow.ClusterID,
			ServiceID:      vmServiceID(vmRow),
		}
		if svc := vmRow.Edges.Service; svc != nil {
			ctxRow.ServiceID = svc.ID
			ctxRow.ServiceName = svc.Name
			if svc.Edges.System != nil {
				ctxRow.SystemID = svc.Edges.System.ID
				ctxRow.SystemName = svc.Edges.System.Name
			}
		}
		byVMID[vmRow.ID] = ctxRow
		if vmRow.ClusterID != "" {
			clusterIDSet[vmRow.ClusterID] = struct{}{}
		}
		if vmRow.TicketID != "" {
			createTicketIDToVMID[vmRow.TicketID] = vmRow.ID
		}
	}

	if len(clusterIDSet) > 0 {
		clusterIDs := sortedStringSet(clusterIDSet)
		clusters, clusterErr := s.client.Cluster.Query().
			Where(entcluster.IDIn(clusterIDs...)).
			All(ctx)
		if clusterErr != nil {
			logger.Warn("failed to fetch clusters for approval summary", zap.Error(clusterErr))
		} else {
			for _, cl := range clusters {
				ctxRow, ok := byVMIDByCluster(cl.ID, byVMID)
				if !ok {
					continue
				}
				for _, vmID := range ctxRow {
					entry := byVMID[vmID]
					entry.ClusterName = firstNonEmptyString(cl.DisplayName, cl.Name, cl.ID)
					entry.ClusterEnvironment = string(cl.Environment)
					byVMID[vmID] = entry
				}
			}
		}
	}

	extraTemplateIDs, extraInstanceSizeIDs = s.loadApprovalVMCreationShape(
		ctx,
		createTicketIDToVMID,
		byVMID,
	)
	return byVMID, extraTemplateIDs, extraInstanceSizeIDs
}

func byVMIDByCluster(
	clusterID string,
	vmByID map[string]approvalVMContext,
) ([]string, bool) {
	vmIDs := make([]string, 0)
	for vmID := range vmByID {
		if vmByID[vmID].ClusterID == clusterID {
			vmIDs = append(vmIDs, vmID)
		}
	}
	if len(vmIDs) == 0 {
		return nil, false
	}
	return vmIDs, true
}

func (s *Server) loadApprovalVMCreationShape(
	ctx context.Context,
	createTicketIDToVMID map[string]string,
	vmByID map[string]approvalVMContext,
) (templateIDs, instanceSizeIDs []string) {
	if len(createTicketIDToVMID) == 0 {
		return nil, nil
	}

	createTicketIDs := make([]string, 0, len(createTicketIDToVMID))
	for ticketID := range createTicketIDToVMID {
		createTicketIDs = append(createTicketIDs, ticketID)
	}
	slices.Sort(createTicketIDs)

	tickets, err := s.client.Ticket.Query().
		Where(entticket.IDIn(createTicketIDs...)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch create tickets for approval VM summary", zap.Error(err))
		return nil, nil
	}

	eventIDToVMID := make(map[string]string, len(tickets))
	eventIDs := make([]string, 0, len(tickets))
	for _, ticket := range tickets {
		vmID := createTicketIDToVMID[ticket.ID]
		if vmID == "" || ticket.EventID == "" {
			continue
		}
		eventIDToVMID[ticket.EventID] = vmID
		eventIDs = append(eventIDs, ticket.EventID)
	}
	if len(eventIDs) == 0 {
		return nil, nil
	}

	events, err := s.client.DomainEvent.Query().
		Where(domainevent.IDIn(eventIDs...)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch create events for approval VM summary", zap.Error(err))
		return nil, nil
	}

	templateIDSet := make(map[string]struct{})
	instanceSizeIDSet := make(map[string]struct{})
	for _, event := range events {
		vmID := eventIDToVMID[event.ID]
		if vmID == "" {
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}

		entry := vmByID[vmID]
		if templateID := trimPayloadString(payload["template_id"]); templateID != "" {
			entry.TemplateID = templateID
			templateIDSet[templateID] = struct{}{}
		}
		if instanceSizeID := trimPayloadString(payload["instance_size_id"]); instanceSizeID != "" {
			entry.InstanceSizeID = instanceSizeID
			instanceSizeIDSet[instanceSizeID] = struct{}{}
		}
		if entry.Namespace == "" {
			entry.Namespace = trimPayloadString(payload["namespace"])
		}
		vmByID[vmID] = entry
	}

	return sortedStringSet(templateIDSet), sortedStringSet(instanceSizeIDSet)
}

func buildTicketSummary(
	ticket *ent.Ticket,
	payload map[string]interface{},
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
	serviceByID map[string]approvalServiceLookup,
	vmByID map[string]approvalVMContext,
) *generated.TicketSummary {
	if ticket == nil {
		return nil
	}

	var summary generated.TicketSummary
	summary.Irreversible = ticket.OperationType == entticket.OperationTypeDELETE

	switch ticket.OperationType {
	case entticket.OperationTypeCREATE:
		items := buildApprovalCreateItemSummaries(payload, templateByID, instanceSizeByID, serviceByID)
		if len(items) > 0 {
			applyApprovalSummaryCommonValues(&summary, items)
			summary.BatchCount = len(items)
			if len(items) > 1 {
				summary.Items = items
			}
		} else {
			applyApprovalSummaryItem(&summary, buildApprovalCreateItemSummary(
				payload,
				templateByID,
				instanceSizeByID,
				serviceByID,
			))
			summary.BatchCount = max(1, trimPayloadPositiveInt(payload["batch_item_count"]))
		}
		if summary.ClusterId == "" {
			summary.ClusterId = trimPayloadString(ticket.PlacementEvaluation["selected_cluster_id"])
		}
		if summary.ClusterName == "" {
			summary.ClusterName = trimPayloadString(ticket.PlacementEvaluation["selected_cluster_name"])
		}
		if summary.ClusterEnvironment == "" {
			summary.ClusterEnvironment = trimPayloadString(ticket.PlacementEvaluation["selected_cluster_environment"])
		}
	case entticket.OperationTypeDELETE, entticket.OperationTypeMODIFY, entticket.OperationTypePOWER:
		items := buildApprovalVMTargetItemSummaries(payload, templateByID, instanceSizeByID, vmByID)
		if len(items) > 0 {
			applyApprovalSummaryCommonValues(&summary, items)
			summary.BatchCount = len(items)
			if len(items) > 1 {
				summary.Items = items
			}
		} else {
			applyApprovalSummaryItem(&summary, buildApprovalVMTargetItemSummary(
				payload,
				templateByID,
				instanceSizeByID,
				vmByID,
			))
			summary.BatchCount = 1
		}
	case entticket.OperationTypeVNC_ACCESS:
		applyApprovalSummaryItem(&summary, buildApprovalVMTargetItemSummary(
			payload,
			templateByID,
			instanceSizeByID,
			vmByID,
		))
		summary.BatchCount = 1
	default:
		return nil
	}

	if !approvalSummaryHasContent(summary) {
		return nil
	}
	return &summary
}

func buildApprovalCreateItemSummaries(
	payload map[string]interface{},
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
	serviceByID map[string]approvalServiceLookup,
) []generated.TicketItemSummary {
	items, ok := payload["items"].([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}

	out := make([]generated.TicketItemSummary, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, buildApprovalCreateItemSummary(item, templateByID, instanceSizeByID, serviceByID))
	}
	return out
}

func buildApprovalCreateItemSummary(
	payload map[string]interface{},
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
	serviceByID map[string]approvalServiceLookup,
) generated.TicketItemSummary {
	summary := generated.TicketItemSummary{
		Namespace:      trimPayloadString(payload["namespace"]),
		TemplateId:     trimPayloadString(payload["template_id"]),
		InstanceSizeId: trimPayloadString(payload["instance_size_id"]),
		ServiceId:      trimPayloadString(payload["service_id"]),
	}
	if lookup, ok := serviceByID[summary.ServiceId]; ok {
		summary.ServiceName = lookup.ServiceName
		summary.SystemId = lookup.SystemID
		summary.SystemName = lookup.SystemName
	}
	if tpl, ok := templateByID[summary.TemplateId]; ok && tpl != nil {
		summary.TemplateName = firstNonEmptyString(tpl.DisplayName, tpl.Name, tpl.ID)
	}
	if size, ok := instanceSizeByID[summary.InstanceSizeId]; ok && size != nil {
		summary.InstanceSizeName = firstNonEmptyString(size.DisplayName, size.Name, size.ID)
		summary.TargetCpuCores = size.CPUCores
		summary.TargetMemoryGi = size.MemoryGi
		if size.DiskGB != 0 {
			summary.TargetDiskGb = size.DiskGB
		}
	}
	return summary
}

func buildApprovalVMTargetItemSummaries(
	payload map[string]interface{},
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
	vmByID map[string]approvalVMContext,
) []generated.TicketItemSummary {
	items, ok := payload["items"].([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}

	out := make([]generated.TicketItemSummary, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, buildApprovalVMTargetItemSummary(item, templateByID, instanceSizeByID, vmByID))
	}
	return out
}

func buildApprovalVMTargetItemSummary(
	payload map[string]interface{},
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
	vmByID map[string]approvalVMContext,
) generated.TicketItemSummary {
	vmID := trimPayloadString(payload["vm_id"])
	vmCtx := vmByID[vmID]

	summary := generated.TicketItemSummary{
		VmId:               firstNonEmptyString(vmCtx.VMID, vmID),
		VmName:             firstNonEmptyString(vmCtx.VMName, trimPayloadString(payload["vm_name"])),
		RequestVmStatus:    firstNonEmptyString(trimPayloadString(payload["request_vm_status"]), trimPayloadString(payload["vm_status"])),
		LatestVmStatus:     firstNonEmptyString(vmCtx.LatestVMStatus, trimPayloadString(payload["latest_vm_status"])),
		SystemId:           vmCtx.SystemID,
		SystemName:         vmCtx.SystemName,
		ServiceId:          vmCtx.ServiceID,
		ServiceName:        vmCtx.ServiceName,
		Namespace:          firstNonEmptyString(vmCtx.Namespace, trimPayloadString(payload["namespace"])),
		ClusterId:          firstNonEmptyString(vmCtx.ClusterID, trimPayloadString(payload["cluster_id"])),
		ClusterName:        firstNonEmptyString(vmCtx.ClusterName, trimPayloadString(payload["cluster_name"])),
		ClusterEnvironment: firstNonEmptyString(vmCtx.ClusterEnvironment, trimPayloadString(payload["cluster_environment"])),
		TemplateId:         vmCtx.TemplateID,
		InstanceSizeId:     vmCtx.InstanceSizeID,
		PowerAction:        trimPayloadString(payload["operation"]),
	}
	summary.VmStatus = summary.LatestVmStatus
	if tpl, ok := templateByID[summary.TemplateId]; ok && tpl != nil {
		summary.TemplateName = firstNonEmptyString(tpl.DisplayName, tpl.Name, tpl.ID)
	}
	if size, ok := instanceSizeByID[summary.InstanceSizeId]; ok && size != nil {
		summary.InstanceSizeName = firstNonEmptyString(size.DisplayName, size.Name, size.ID)
		if summary.CurrentCpuCores == 0 {
			summary.CurrentCpuCores = size.CPUCores
		}
		if summary.CurrentMemoryGi == 0 {
			summary.CurrentMemoryGi = size.MemoryGi
		}
		if summary.CurrentDiskGb == 0 && size.DiskGB != 0 {
			summary.CurrentDiskGb = size.DiskGB
		}
	}

	if currentCPU := payloadNumberFloat(payload["current_cpu_cores"]); currentCPU > 0 {
		summary.CurrentCpuCores = currentCPU
	}
	if currentMemory := payloadNumberFloat(payload["current_memory_gi"]); currentMemory > 0 {
		summary.CurrentMemoryGi = currentMemory
	}
	if currentDisk := trimPayloadPositiveInt(payload["current_disk_gb"]); currentDisk > 0 {
		summary.CurrentDiskGb = currentDisk
	}
	if targetCPU := payloadNumberFloat(payload["target_cpu_cores"]); targetCPU > 0 {
		summary.TargetCpuCores = targetCPU
	}
	if targetMemory := payloadNumberFloat(payload["target_memory_gi"]); targetMemory > 0 {
		summary.TargetMemoryGi = targetMemory
	}
	if targetDisk := trimPayloadPositiveInt(payload["target_disk_gb"]); targetDisk > 0 {
		summary.TargetDiskGb = targetDisk
	}

	return summary
}

func applyApprovalSummaryCommonValues(
	summary *generated.TicketSummary,
	items []generated.TicketItemSummary,
) {
	if summary == nil || len(items) == 0 {
		return
	}
	summary.SystemId = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.SystemId })
	summary.SystemName = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.SystemName })
	summary.ServiceId = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.ServiceId })
	summary.ServiceName = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.ServiceName })
	summary.Namespace = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.Namespace })
	summary.ClusterId = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.ClusterId })
	summary.ClusterName = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.ClusterName })
	summary.ClusterEnvironment = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.ClusterEnvironment })
	summary.VmId = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.VmId })
	summary.VmName = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.VmName })
	summary.RequestVmStatus = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.RequestVmStatus })
	summary.LatestVmStatus = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.LatestVmStatus })
	summary.VmStatus = summary.LatestVmStatus
	summary.TemplateId = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.TemplateId })
	summary.TemplateName = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.TemplateName })
	summary.InstanceSizeId = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.InstanceSizeId })
	summary.InstanceSizeName = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.InstanceSizeName })
	summary.PowerAction = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.PowerAction })
	summary.CurrentCpuCores = commonApprovalItemFloat(items, func(item generated.TicketItemSummary) float64 { return item.CurrentCpuCores })
	summary.CurrentMemoryGi = commonApprovalItemFloat(items, func(item generated.TicketItemSummary) float64 { return item.CurrentMemoryGi })
	summary.CurrentDiskGb = commonApprovalItemInt(items, func(item generated.TicketItemSummary) int { return item.CurrentDiskGb })
	summary.TargetCpuCores = commonApprovalItemFloat(items, func(item generated.TicketItemSummary) float64 { return item.TargetCpuCores })
	summary.TargetMemoryGi = commonApprovalItemFloat(items, func(item generated.TicketItemSummary) float64 { return item.TargetMemoryGi })
	summary.TargetDiskGb = commonApprovalItemInt(items, func(item generated.TicketItemSummary) int { return item.TargetDiskGb })
}

func applyApprovalSummaryItem(
	summary *generated.TicketSummary,
	item generated.TicketItemSummary,
) {
	if summary == nil {
		return
	}
	summary.SystemId = item.SystemId
	summary.SystemName = item.SystemName
	summary.ServiceId = item.ServiceId
	summary.ServiceName = item.ServiceName
	summary.Namespace = item.Namespace
	summary.ClusterId = item.ClusterId
	summary.ClusterName = item.ClusterName
	summary.ClusterEnvironment = item.ClusterEnvironment
	summary.VmId = item.VmId
	summary.VmName = item.VmName
	summary.RequestVmStatus = item.RequestVmStatus
	summary.LatestVmStatus = item.LatestVmStatus
	summary.VmStatus = item.LatestVmStatus
	summary.TemplateId = item.TemplateId
	summary.TemplateName = item.TemplateName
	summary.InstanceSizeId = item.InstanceSizeId
	summary.InstanceSizeName = item.InstanceSizeName
	summary.CurrentCpuCores = item.CurrentCpuCores
	summary.CurrentMemoryGi = item.CurrentMemoryGi
	summary.CurrentDiskGb = item.CurrentDiskGb
	summary.TargetCpuCores = item.TargetCpuCores
	summary.TargetMemoryGi = item.TargetMemoryGi
	summary.TargetDiskGb = item.TargetDiskGb
	summary.PowerAction = item.PowerAction
}

func commonApprovalItemString(
	items []generated.TicketItemSummary,
	getter func(generated.TicketItemSummary) string,
) string {
	if len(items) == 0 {
		return ""
	}
	first := strings.TrimSpace(getter(items[0]))
	if first == "" {
		return ""
	}
	for i := 1; i < len(items); i++ {
		if strings.TrimSpace(getter(items[i])) != first {
			return ""
		}
	}
	return first
}

func commonApprovalItemFloat(
	items []generated.TicketItemSummary,
	getter func(generated.TicketItemSummary) float64,
) float64 {
	if len(items) == 0 {
		return 0
	}
	first := getter(items[0])
	if first <= 0 {
		return 0
	}
	for i := 1; i < len(items); i++ {
		if getter(items[i]) != first {
			return 0
		}
	}
	return first
}

func commonApprovalItemInt(
	items []generated.TicketItemSummary,
	getter func(generated.TicketItemSummary) int,
) int {
	if len(items) == 0 {
		return 0
	}
	first := getter(items[0])
	if first <= 0 {
		return 0
	}
	for i := 1; i < len(items); i++ {
		if getter(items[i]) != first {
			return 0
		}
	}
	return first
}

func approvalSummaryHasContent(summary generated.TicketSummary) bool {
	return summary.SystemId != "" ||
		summary.SystemName != "" ||
		summary.ServiceId != "" ||
		summary.ServiceName != "" ||
		summary.Namespace != "" ||
		summary.ClusterId != "" ||
		summary.ClusterName != "" ||
		summary.ClusterEnvironment != "" ||
		summary.VmId != "" ||
		summary.VmName != "" ||
		summary.RequestVmStatus != "" ||
		summary.LatestVmStatus != "" ||
		summary.VmStatus != "" ||
		summary.TemplateId != "" ||
		summary.TemplateName != "" ||
		summary.InstanceSizeId != "" ||
		summary.InstanceSizeName != "" ||
		summary.BatchCount > 0 ||
		summary.CurrentCpuCores > 0 ||
		summary.CurrentMemoryGi > 0 ||
		summary.CurrentDiskGb > 0 ||
		summary.TargetCpuCores > 0 ||
		summary.TargetMemoryGi > 0 ||
		summary.TargetDiskGb > 0 ||
		summary.PowerAction != "" ||
		summary.Irreversible ||
		len(summary.Items) > 0
}

func vmServiceID(vmRow *ent.VM) string {
	if vmRow == nil || vmRow.Edges.Service == nil {
		return ""
	}
	return vmRow.Edges.Service.ID
}

func payloadNumberFloat(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return v
		}
	case int:
		if v > 0 {
			return float64(v)
		}
	}
	return 0
}

func sliceToStringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}
