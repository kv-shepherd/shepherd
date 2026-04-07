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
	ServiceID     string
	ServiceName   string
	SystemID      string
	SystemName    string
	SystemOwnerID string
}

type approvalVMContext struct {
	VMID               string
	VMName             string
	LatestVMStatus     string
	OwnerID            string
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
			lookup.SystemOwnerID = strings.TrimSpace(svc.Edges.System.CreatedBy)
		}
		byServiceID[svc.ID] = lookup
	}
	return byServiceID
}

func (s *Server) loadApprovalAllServiceLookups(
	ctx context.Context,
) map[string]approvalServiceLookup {
	byServiceID := make(map[string]approvalServiceLookup)

	services, err := s.client.Service.Query().
		WithSystem().
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch full service catalog for approval summary fallback", zap.Error(err))
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
			lookup.SystemOwnerID = strings.TrimSpace(svc.Edges.System.CreatedBy)
		}
		byServiceID[svc.ID] = lookup
	}

	return byServiceID
}

func (s *Server) loadApprovalVMContexts(
	ctx context.Context,
	vmIDs []string,
	serviceByID map[string]approvalServiceLookup,
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
			OwnerID:        strings.TrimSpace(vmRow.CreatedBy),
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

	historyTemplateIDs, historyInstanceSizeIDs := s.loadApprovalHistoricalVMContexts(
		ctx,
		vmIDs,
		byVMID,
		serviceByID,
	)
	extraTemplateIDs = append(extraTemplateIDs, historyTemplateIDs...)
	extraInstanceSizeIDs = append(extraInstanceSizeIDs, historyInstanceSizeIDs...)

	clusterIDSet = make(map[string]struct{})
	for vmID := range byVMID {
		entry := byVMID[vmID]
		if strings.TrimSpace(entry.ClusterID) != "" {
			clusterIDSet[entry.ClusterID] = struct{}{}
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

	createTemplateIDs, createInstanceSizeIDs := s.loadApprovalVMCreationShape(
		ctx,
		createTicketIDToVMID,
		byVMID,
	)
	extraTemplateIDs = append(extraTemplateIDs, createTemplateIDs...)
	extraInstanceSizeIDs = append(extraInstanceSizeIDs, createInstanceSizeIDs...)
	return byVMID, extraTemplateIDs, extraInstanceSizeIDs
}

func (s *Server) loadApprovalHistoricalVMContexts(
	ctx context.Context,
	vmIDs []string,
	byVMID map[string]approvalVMContext,
	serviceByID map[string]approvalServiceLookup,
) (templateIDs, instanceSizeIDs []string) {
	missingVMIDs := make([]string, 0, len(vmIDs))
	for _, vmID := range vmIDs {
		if strings.TrimSpace(vmID) == "" {
			continue
		}
		entry := byVMID[vmID]
		if approvalVMContextHasReadableScope(entry) {
			continue
		}
		missingVMIDs = append(missingVMIDs, vmID)
	}
	if len(missingVMIDs) == 0 {
		return nil, nil
	}

	events, err := s.client.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("vm"),
			domainevent.AggregateIDIn(missingVMIDs...),
		).
		Order(ent.Asc(domainevent.FieldAggregateID), ent.Desc(domainevent.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch historical VM events for approval summary fallback", zap.Error(err))
		return nil, nil
	}
	if len(events) == 0 {
		return nil, nil
	}

	templateIDSet := make(map[string]struct{})
	instanceSizeIDSet := make(map[string]struct{})
	inferenceCatalog := serviceByID

	for _, event := range events {
		if event == nil || strings.TrimSpace(event.AggregateID) == "" || len(event.Payload) == 0 {
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			logger.Warn("failed to decode historical VM event for approval summary fallback",
				zap.String("event_id", event.ID),
				zap.String("aggregate_id", event.AggregateID),
				zap.Error(err),
			)
			continue
		}

		entry := byVMID[event.AggregateID]
		templateID, instanceSizeID := mergeApprovalVMContextFromPayload(&entry, payload)
		if templateID != "" {
			templateIDSet[templateID] = struct{}{}
		}
		if instanceSizeID != "" {
			instanceSizeIDSet[instanceSizeID] = struct{}{}
		}

		if !approvalVMContextHasReadableScope(entry) && strings.TrimSpace(entry.VMName) != "" {
			lookup, ok := inferApprovalScopeFromVMName(entry.Namespace, entry.VMName, inferenceCatalog)
			if !ok {
				inferenceCatalog = s.loadApprovalAllServiceLookups(ctx)
				lookup, ok = inferApprovalScopeFromVMName(entry.Namespace, entry.VMName, inferenceCatalog)
			}
			if ok {
				mergeApprovalServiceLookupIntoVMContext(&entry, lookup)
			}
		}

		byVMID[event.AggregateID] = entry
	}

	return sortedStringSet(templateIDSet), sortedStringSet(instanceSizeIDSet)
}

func approvalVMContextHasReadableScope(entry approvalVMContext) bool {
	return strings.TrimSpace(entry.SystemName) != "" &&
		strings.TrimSpace(entry.ServiceName) != "" &&
		strings.TrimSpace(entry.OwnerID) != ""
}

func mergeApprovalVMContextFromPayload(
	entry *approvalVMContext,
	payload map[string]interface{},
) (templateID, instanceSizeID string) {
	if entry == nil {
		return "", ""
	}

	if entry.VMID == "" {
		entry.VMID = trimPayloadString(payload["vm_id"])
	}
	if entry.VMName == "" {
		entry.VMName = trimPayloadString(payload["vm_name"])
	}
	if entry.LatestVMStatus == "" {
		entry.LatestVMStatus = firstNonEmptyString(
			trimPayloadString(payload["latest_vm_status"]),
			trimPayloadString(payload["vm_status"]),
			trimPayloadString(payload["request_vm_status"]),
		)
	}
	if entry.OwnerID == "" {
		entry.OwnerID = firstNonEmptyString(
			trimPayloadString(payload["owner_id"]),
			trimPayloadString(payload["requester_id"]),
			trimPayloadString(payload["created_by"]),
			trimPayloadString(payload["actor"]),
		)
	}
	if entry.Namespace == "" {
		entry.Namespace = trimPayloadString(payload["namespace"])
	}
	if entry.ClusterID == "" {
		entry.ClusterID = trimPayloadString(payload["cluster_id"])
	}
	if entry.ClusterName == "" {
		entry.ClusterName = trimPayloadString(payload["cluster_name"])
	}
	if entry.ClusterEnvironment == "" {
		entry.ClusterEnvironment = trimPayloadString(payload["cluster_environment"])
	}
	if entry.ServiceID == "" {
		entry.ServiceID = trimPayloadString(payload["service_id"])
	}
	if entry.ServiceName == "" {
		entry.ServiceName = trimPayloadString(payload["service_name"])
	}
	if entry.SystemID == "" {
		entry.SystemID = trimPayloadString(payload["system_id"])
	}
	if entry.SystemName == "" {
		entry.SystemName = trimPayloadString(payload["system_name"])
	}
	if entry.TemplateID == "" {
		entry.TemplateID = trimPayloadString(payload["template_id"])
	}
	if entry.InstanceSizeID == "" {
		entry.InstanceSizeID = trimPayloadString(payload["instance_size_id"])
	}
	if entry.CurrentCPUCores == 0 {
		entry.CurrentCPUCores = firstPositiveFloat(
			payloadNumberFloat(payload["current_cpu_cores"]),
			payloadNumberFloat(payload["target_cpu_cores"]),
		)
	}
	if entry.CurrentMemoryGi == 0 {
		entry.CurrentMemoryGi = firstPositiveFloat(
			payloadNumberFloat(payload["current_memory_gi"]),
			payloadNumberFloat(payload["target_memory_gi"]),
		)
	}
	if entry.CurrentDiskGB == 0 {
		entry.CurrentDiskGB = firstPositiveInt(
			trimPayloadPositiveInt(payload["current_disk_gb"]),
			trimPayloadPositiveInt(payload["target_disk_gb"]),
		)
	}

	return entry.TemplateID, entry.InstanceSizeID
}

func inferApprovalScopeFromVMName(
	namespace string,
	vmName string,
	serviceByID map[string]approvalServiceLookup,
) (approvalServiceLookup, bool) {
	trimmedNamespace := strings.TrimSpace(namespace)
	trimmedVMName := strings.TrimSpace(vmName)
	if trimmedVMName == "" || len(serviceByID) == 0 {
		return approvalServiceLookup{}, false
	}

	var (
		bestMatch approvalServiceLookup
		bestLen   int
	)
	for _, lookup := range serviceByID {
		systemName := strings.TrimSpace(lookup.SystemName)
		serviceName := strings.TrimSpace(lookup.ServiceName)
		if systemName == "" || serviceName == "" {
			continue
		}

		prefix := systemName + "-" + serviceName
		if trimmedNamespace != "" {
			prefix = trimmedNamespace + "-" + prefix
		}
		if trimmedVMName != prefix && !strings.HasPrefix(trimmedVMName, prefix+"-") {
			continue
		}
		if len(prefix) <= bestLen {
			continue
		}
		bestMatch = lookup
		bestLen = len(prefix)
	}
	if bestLen == 0 {
		return approvalServiceLookup{}, false
	}
	return bestMatch, true
}

func mergeApprovalServiceLookupIntoVMContext(
	entry *approvalVMContext,
	lookup approvalServiceLookup,
) {
	if entry == nil {
		return
	}
	if entry.ServiceID == "" {
		entry.ServiceID = lookup.ServiceID
	}
	if entry.ServiceName == "" {
		entry.ServiceName = lookup.ServiceName
	}
	if entry.SystemID == "" {
		entry.SystemID = lookup.SystemID
	}
	if entry.SystemName == "" {
		entry.SystemName = lookup.SystemName
	}
	if entry.OwnerID == "" {
		entry.OwnerID = lookup.SystemOwnerID
	}
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
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

func (s *Server) loadApprovalBatchChildFallbackItems(
	ctx context.Context,
	tickets []*ent.Ticket,
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
	serviceByID map[string]approvalServiceLookup,
	vmByID map[string]approvalVMContext,
	actorByID map[string]approvalActorLookup,
) map[string][]generated.TicketItemSummary {
	parentIDSet := make(map[string]struct{})
	for _, ticket := range tickets {
		if ticket == nil || strings.TrimSpace(ticket.ID) == "" {
			continue
		}
		parentIDSet[ticket.ID] = struct{}{}
	}
	if len(parentIDSet) == 0 {
		return map[string][]generated.TicketItemSummary{}
	}

	parentIDs := sortedStringSet(parentIDSet)
	children, err := s.client.Ticket.Query().
		Where(entticket.ParentTicketIDIn(parentIDs...)).
		Order(ent.Asc(entticket.FieldParentTicketID), ent.Asc(entticket.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch child tickets for batch audit summary fallback", zap.Error(err))
		return map[string][]generated.TicketItemSummary{}
	}
	if len(children) == 0 {
		return map[string][]generated.TicketItemSummary{}
	}

	eventIDs := make([]string, 0, len(children))
	for _, child := range children {
		if child == nil || strings.TrimSpace(child.EventID) == "" {
			continue
		}
		eventIDs = append(eventIDs, child.EventID)
	}
	eventPayloadByID := make(map[string][]byte, len(eventIDs))
	if len(eventIDs) > 0 {
		events, eventErr := s.client.DomainEvent.Query().
			Where(domainevent.IDIn(eventIDs...)).
			All(ctx)
		if eventErr != nil {
			logger.Warn("failed to fetch child ticket events for batch audit summary fallback", zap.Error(eventErr))
			return map[string][]generated.TicketItemSummary{}
		}
		for _, event := range events {
			if event == nil {
				continue
			}
			eventPayloadByID[event.ID] = event.Payload
		}
	}

	childTemplateIDSet := make(map[string]struct{})
	childInstanceSizeIDSet := make(map[string]struct{})
	childServiceIDSet := make(map[string]struct{})
	childActorIDSet := make(map[string]struct{})
	decodedPayloadByEventID := make(map[string]map[string]interface{}, len(eventPayloadByID))
	for _, child := range children {
		if child == nil {
			continue
		}
		if requesterID := strings.TrimSpace(child.Requester); requesterID != "" {
			childActorIDSet[requesterID] = struct{}{}
		}
		if approverID := strings.TrimSpace(child.Approver); approverID != "" {
			childActorIDSet[approverID] = struct{}{}
		}
		raw := eventPayloadByID[child.EventID]
		if len(raw) == 0 {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			logger.Warn("failed to decode child ticket payload for batch audit summary fallback",
				zap.String("ticket_id", child.ID),
				zap.String("event_id", child.EventID),
				zap.Error(err),
			)
			continue
		}
		decodedPayloadByEventID[child.EventID] = payload
		collectCatalogIDsFromPayload(payload, childTemplateIDSet, childInstanceSizeIDSet)
		collectServiceIDsFromPayload(payload, childServiceIDSet)
		collectApprovalActorIDsFromPayload(payload, childActorIDSet)
	}

	resolvedTemplateByID := make(map[string]*ent.Template, len(templateByID))
	for key, value := range templateByID {
		resolvedTemplateByID[key] = value
	}
	resolvedInstanceSizeByID := make(map[string]*ent.InstanceSize, len(instanceSizeByID))
	for key, value := range instanceSizeByID {
		resolvedInstanceSizeByID[key] = value
	}
	missingTemplateIDs := make(map[string]struct{})
	for templateID := range childTemplateIDSet {
		if _, ok := resolvedTemplateByID[templateID]; !ok {
			missingTemplateIDs[templateID] = struct{}{}
		}
	}
	missingInstanceSizeIDs := make(map[string]struct{})
	for instanceSizeID := range childInstanceSizeIDSet {
		if _, ok := resolvedInstanceSizeByID[instanceSizeID]; !ok {
			missingInstanceSizeIDs[instanceSizeID] = struct{}{}
		}
	}
	if len(missingTemplateIDs) > 0 || len(missingInstanceSizeIDs) > 0 {
		extraTemplateByID, extraInstanceSizeByID := s.loadApprovalCatalogLookups(
			ctx,
			sortedStringSet(missingTemplateIDs),
			sortedStringSet(missingInstanceSizeIDs),
		)
		for key, value := range extraTemplateByID {
			resolvedTemplateByID[key] = value
		}
		for key, value := range extraInstanceSizeByID {
			resolvedInstanceSizeByID[key] = value
		}
	}

	resolvedServiceByID := make(map[string]approvalServiceLookup, len(serviceByID))
	for key, value := range serviceByID {
		resolvedServiceByID[key] = value
	}
	missingServiceIDs := make(map[string]struct{})
	for serviceID := range childServiceIDSet {
		if _, ok := resolvedServiceByID[serviceID]; !ok {
			missingServiceIDs[serviceID] = struct{}{}
		}
	}
	if len(missingServiceIDs) > 0 {
		for key, value := range s.loadApprovalServiceLookups(ctx, sortedStringSet(missingServiceIDs)) {
			resolvedServiceByID[key] = value
		}
	}

	resolvedActorByID := make(map[string]approvalActorLookup, len(actorByID))
	for key, value := range actorByID {
		resolvedActorByID[key] = value
	}
	missingActorIDs := make(map[string]struct{})
	for actorID := range childActorIDSet {
		if _, ok := resolvedActorByID[actorID]; !ok {
			missingActorIDs[actorID] = struct{}{}
		}
	}
	if len(missingActorIDs) > 0 {
		for key, value := range s.loadApprovalActorLookupsByIDs(ctx, sortedStringSet(missingActorIDs)) {
			resolvedActorByID[key] = value
		}
	}

	byParentID := make(map[string][]generated.TicketItemSummary)
	for _, child := range children {
		if child == nil || strings.TrimSpace(child.ParentTicketID) == "" {
			continue
		}
		payload := decodedPayloadByEventID[child.EventID]
		if payload == nil {
			payload = map[string]interface{}{}
		}
		summary := buildTicketSummary(
			child,
			payload,
			resolvedTemplateByID,
			resolvedInstanceSizeByID,
			resolvedServiceByID,
			vmByID,
			resolvedActorByID,
			nil,
		)
		if summary == nil {
			continue
		}
		byParentID[child.ParentTicketID] = append(
			byParentID[child.ParentTicketID],
			ticketSummaryToItemSummary(summary),
		)
	}
	return byParentID
}

func buildTicketSummary(
	ticket *ent.Ticket,
	payload map[string]interface{},
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
	serviceByID map[string]approvalServiceLookup,
	vmByID map[string]approvalVMContext,
	actorByID map[string]approvalActorLookup,
	batchFallbackItems []generated.TicketItemSummary,
) *generated.TicketSummary {
	if ticket == nil {
		return nil
	}

	var summary generated.TicketSummary
	summary.Irreversible = ticket.OperationType == entticket.OperationTypeDELETE
	requesterDisplayName, requesterUsername := approvalActorIdentity(ticket.Requester, actorByID)

	switch ticket.OperationType {
	case entticket.OperationTypeCREATE:
		items := buildApprovalCreateItemSummaries(
			payload,
			templateByID,
			instanceSizeByID,
			serviceByID,
			requesterDisplayName,
			requesterUsername,
		)
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
				requesterDisplayName,
				requesterUsername,
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
		items := buildApprovalVMTargetItemSummaries(
			payload,
			templateByID,
			instanceSizeByID,
			vmByID,
			actorByID,
		)
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
				actorByID,
			))
			summary.BatchCount = 1
		}
	case entticket.OperationTypeVNC_ACCESS:
		applyApprovalSummaryItem(&summary, buildApprovalVMTargetItemSummary(
			payload,
			templateByID,
			instanceSizeByID,
			vmByID,
			actorByID,
		))
		summary.BatchCount = 1
	default:
		return nil
	}

	mergeBatchFallbackItems(&summary, batchFallbackItems)

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
	ownerDisplayName string,
	ownerUsername string,
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
		out = append(out, buildApprovalCreateItemSummary(
			item,
			templateByID,
			instanceSizeByID,
			serviceByID,
			ownerDisplayName,
			ownerUsername,
		))
	}
	return out
}

func buildApprovalCreateItemSummary(
	payload map[string]interface{},
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
	serviceByID map[string]approvalServiceLookup,
	ownerDisplayName string,
	ownerUsername string,
) generated.TicketItemSummary {
	summary := generated.TicketItemSummary{
		SystemId:         trimPayloadString(payload["system_id"]),
		SystemName:       trimPayloadString(payload["system_name"]),
		Namespace:        trimPayloadString(payload["namespace"]),
		TemplateId:       trimPayloadString(payload["template_id"]),
		TemplateName:     trimPayloadString(payload["template_name"]),
		InstanceSizeId:   trimPayloadString(payload["instance_size_id"]),
		InstanceSizeName: trimPayloadString(payload["instance_size_name"]),
		ServiceId:        trimPayloadString(payload["service_id"]),
		ServiceName:      trimPayloadString(payload["service_name"]),
		OwnerDisplayName: firstNonEmptyString(trimPayloadString(payload["owner_display_name"]), strings.TrimSpace(ownerDisplayName)),
		OwnerUsername:    firstNonEmptyString(trimPayloadString(payload["owner_username"]), strings.TrimSpace(ownerUsername)),
		TargetCpuCores:   payloadNumberFloat(payload["target_cpu_cores"]),
		TargetMemoryGi:   payloadNumberFloat(payload["target_memory_gi"]),
		TargetDiskGb:     trimPayloadPositiveInt(payload["target_disk_gb"]),
	}
	if lookup, ok := serviceByID[summary.ServiceId]; ok {
		summary.ServiceName = firstNonEmptyString(lookup.ServiceName, summary.ServiceName)
		summary.SystemId = firstNonEmptyString(lookup.SystemID, summary.SystemId)
		summary.SystemName = firstNonEmptyString(lookup.SystemName, summary.SystemName)
	}
	if tpl, ok := templateByID[summary.TemplateId]; ok && tpl != nil {
		summary.TemplateName = firstNonEmptyString(firstNonEmptyString(tpl.DisplayName, tpl.Name, tpl.ID), summary.TemplateName)
	}
	if size, ok := instanceSizeByID[summary.InstanceSizeId]; ok && size != nil {
		summary.InstanceSizeName = firstNonEmptyString(firstNonEmptyString(size.DisplayName, size.Name, size.ID), summary.InstanceSizeName)
		if summary.TargetCpuCores == 0 {
			summary.TargetCpuCores = size.CPUCores
		}
		if summary.TargetMemoryGi == 0 {
			summary.TargetMemoryGi = size.MemoryGi
		}
		if summary.TargetDiskGb == 0 && size.DiskGB != 0 {
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
	actorByID map[string]approvalActorLookup,
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
		out = append(out, buildApprovalVMTargetItemSummary(
			item,
			templateByID,
			instanceSizeByID,
			vmByID,
			actorByID,
		))
	}
	return out
}

func buildApprovalVMTargetItemSummary(
	payload map[string]interface{},
	templateByID map[string]*ent.Template,
	instanceSizeByID map[string]*ent.InstanceSize,
	vmByID map[string]approvalVMContext,
	actorByID map[string]approvalActorLookup,
) generated.TicketItemSummary {
	vmID := trimPayloadString(payload["vm_id"])
	vmCtx := vmByID[vmID]
	payloadOwnerDisplayName := trimPayloadString(payload["owner_display_name"])
	payloadOwnerUsername := trimPayloadString(payload["owner_username"])
	ownerDisplayName, ownerUsername := approvalActorIdentity(
		firstNonEmptyString(strings.TrimSpace(vmCtx.OwnerID), trimPayloadString(payload["owner_id"])),
		actorByID,
	)
	ownerDisplayName = firstNonEmptyString(payloadOwnerDisplayName, ownerDisplayName)
	ownerUsername = firstNonEmptyString(payloadOwnerUsername, ownerUsername)

	summary := generated.TicketItemSummary{
		VmId:               firstNonEmptyString(vmCtx.VMID, vmID),
		VmName:             firstNonEmptyString(vmCtx.VMName, trimPayloadString(payload["vm_name"])),
		RequestVmStatus:    firstNonEmptyString(trimPayloadString(payload["request_vm_status"]), trimPayloadString(payload["vm_status"])),
		LatestVmStatus:     firstNonEmptyString(vmCtx.LatestVMStatus, trimPayloadString(payload["latest_vm_status"])),
		SystemId:           firstNonEmptyString(vmCtx.SystemID, trimPayloadString(payload["system_id"])),
		SystemName:         firstNonEmptyString(vmCtx.SystemName, trimPayloadString(payload["system_name"])),
		ServiceId:          firstNonEmptyString(vmCtx.ServiceID, trimPayloadString(payload["service_id"])),
		ServiceName:        firstNonEmptyString(vmCtx.ServiceName, trimPayloadString(payload["service_name"])),
		Namespace:          firstNonEmptyString(vmCtx.Namespace, trimPayloadString(payload["namespace"])),
		ClusterId:          firstNonEmptyString(vmCtx.ClusterID, trimPayloadString(payload["cluster_id"])),
		ClusterName:        firstNonEmptyString(vmCtx.ClusterName, trimPayloadString(payload["cluster_name"])),
		ClusterEnvironment: firstNonEmptyString(vmCtx.ClusterEnvironment, trimPayloadString(payload["cluster_environment"])),
		TemplateId:         firstNonEmptyString(vmCtx.TemplateID, trimPayloadString(payload["template_id"])),
		TemplateName:       trimPayloadString(payload["template_name"]),
		InstanceSizeId:     firstNonEmptyString(vmCtx.InstanceSizeID, trimPayloadString(payload["instance_size_id"])),
		InstanceSizeName:   trimPayloadString(payload["instance_size_name"]),
		PowerAction:        trimPayloadString(payload["operation"]),
		OwnerDisplayName:   ownerDisplayName,
		OwnerUsername:      ownerUsername,
		CurrentCpuCores:    vmCtx.CurrentCPUCores,
		CurrentMemoryGi:    vmCtx.CurrentMemoryGi,
		CurrentDiskGb:      vmCtx.CurrentDiskGB,
	}
	summary.VmStatus = summary.LatestVmStatus
	if tpl, ok := templateByID[summary.TemplateId]; ok && tpl != nil {
		summary.TemplateName = firstNonEmptyString(firstNonEmptyString(tpl.DisplayName, tpl.Name, tpl.ID), summary.TemplateName)
	}
	if size, ok := instanceSizeByID[summary.InstanceSizeId]; ok && size != nil {
		summary.InstanceSizeName = firstNonEmptyString(firstNonEmptyString(size.DisplayName, size.Name, size.ID), summary.InstanceSizeName)
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
	summary.OwnerDisplayName = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.OwnerDisplayName })
	summary.OwnerUsername = commonApprovalItemString(items, func(item generated.TicketItemSummary) string { return item.OwnerUsername })
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
	summary.OwnerDisplayName = item.OwnerDisplayName
	summary.OwnerUsername = item.OwnerUsername
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

func mergeBatchFallbackItems(
	summary *generated.TicketSummary,
	fallbackItems []generated.TicketItemSummary,
) {
	if summary == nil || len(fallbackItems) == 0 {
		return
	}

	if len(summary.Items) == 0 {
		summary.Items = fallbackItems
	} else {
		merged := make([]generated.TicketItemSummary, 0, max(len(summary.Items), len(fallbackItems)))
		limit := max(len(summary.Items), len(fallbackItems))
		for idx := 0; idx < limit; idx++ {
			var existing generated.TicketItemSummary
			var fallback generated.TicketItemSummary
			if idx < len(summary.Items) {
				existing = summary.Items[idx]
			}
			if idx < len(fallbackItems) {
				fallback = fallbackItems[idx]
			}
			merged = append(merged, mergeApprovalItemSummary(existing, fallback))
		}
		summary.Items = merged
	}

	summary.BatchCount = max(summary.BatchCount, len(summary.Items))
	var mergedCommon generated.TicketSummary
	applyApprovalSummaryCommonValues(&mergedCommon, summary.Items)
	mergeApprovalSummary(summary, mergedCommon)
}

func mergeApprovalItemSummary(
	current generated.TicketItemSummary,
	fallback generated.TicketItemSummary,
) generated.TicketItemSummary {
	if current.VmId == "" {
		current.VmId = fallback.VmId
	}
	if current.VmName == "" {
		current.VmName = fallback.VmName
	}
	if current.SystemId == "" {
		current.SystemId = fallback.SystemId
	}
	if current.SystemName == "" {
		current.SystemName = fallback.SystemName
	}
	if current.ServiceId == "" {
		current.ServiceId = fallback.ServiceId
	}
	if current.ServiceName == "" {
		current.ServiceName = fallback.ServiceName
	}
	if current.Namespace == "" {
		current.Namespace = fallback.Namespace
	}
	if current.ClusterId == "" {
		current.ClusterId = fallback.ClusterId
	}
	if current.ClusterName == "" {
		current.ClusterName = fallback.ClusterName
	}
	if current.ClusterEnvironment == "" {
		current.ClusterEnvironment = fallback.ClusterEnvironment
	}
	if current.OwnerDisplayName == "" {
		current.OwnerDisplayName = fallback.OwnerDisplayName
	}
	if current.OwnerUsername == "" {
		current.OwnerUsername = fallback.OwnerUsername
	}
	if current.TemplateId == "" {
		current.TemplateId = fallback.TemplateId
	}
	if current.TemplateName == "" {
		current.TemplateName = fallback.TemplateName
	}
	if current.InstanceSizeId == "" {
		current.InstanceSizeId = fallback.InstanceSizeId
	}
	if current.InstanceSizeName == "" {
		current.InstanceSizeName = fallback.InstanceSizeName
	}
	if current.RequestVmStatus == "" {
		current.RequestVmStatus = fallback.RequestVmStatus
	}
	if current.LatestVmStatus == "" {
		current.LatestVmStatus = fallback.LatestVmStatus
	}
	if current.PowerAction == "" {
		current.PowerAction = fallback.PowerAction
	}
	if current.CurrentCpuCores == 0 {
		current.CurrentCpuCores = fallback.CurrentCpuCores
	}
	if current.CurrentMemoryGi == 0 {
		current.CurrentMemoryGi = fallback.CurrentMemoryGi
	}
	if current.CurrentDiskGb == 0 {
		current.CurrentDiskGb = fallback.CurrentDiskGb
	}
	if current.TargetCpuCores == 0 {
		current.TargetCpuCores = fallback.TargetCpuCores
	}
	if current.TargetMemoryGi == 0 {
		current.TargetMemoryGi = fallback.TargetMemoryGi
	}
	if current.TargetDiskGb == 0 {
		current.TargetDiskGb = fallback.TargetDiskGb
	}
	return current
}

func mergeApprovalSummary(
	current *generated.TicketSummary,
	fallback generated.TicketSummary,
) {
	if current == nil {
		return
	}
	if current.SystemId == "" {
		current.SystemId = fallback.SystemId
	}
	if current.SystemName == "" {
		current.SystemName = fallback.SystemName
	}
	if current.ServiceId == "" {
		current.ServiceId = fallback.ServiceId
	}
	if current.ServiceName == "" {
		current.ServiceName = fallback.ServiceName
	}
	if current.Namespace == "" {
		current.Namespace = fallback.Namespace
	}
	if current.ClusterId == "" {
		current.ClusterId = fallback.ClusterId
	}
	if current.ClusterName == "" {
		current.ClusterName = fallback.ClusterName
	}
	if current.ClusterEnvironment == "" {
		current.ClusterEnvironment = fallback.ClusterEnvironment
	}
	if current.VmId == "" {
		current.VmId = fallback.VmId
	}
	if current.VmName == "" {
		current.VmName = fallback.VmName
	}
	if current.OwnerDisplayName == "" {
		current.OwnerDisplayName = fallback.OwnerDisplayName
	}
	if current.OwnerUsername == "" {
		current.OwnerUsername = fallback.OwnerUsername
	}
	if current.RequestVmStatus == "" {
		current.RequestVmStatus = fallback.RequestVmStatus
	}
	if current.LatestVmStatus == "" {
		current.LatestVmStatus = fallback.LatestVmStatus
		current.VmStatus = fallback.LatestVmStatus
	}
	if current.TemplateId == "" {
		current.TemplateId = fallback.TemplateId
	}
	if current.TemplateName == "" {
		current.TemplateName = fallback.TemplateName
	}
	if current.InstanceSizeId == "" {
		current.InstanceSizeId = fallback.InstanceSizeId
	}
	if current.InstanceSizeName == "" {
		current.InstanceSizeName = fallback.InstanceSizeName
	}
	if current.PowerAction == "" {
		current.PowerAction = fallback.PowerAction
	}
	if current.CurrentCpuCores == 0 {
		current.CurrentCpuCores = fallback.CurrentCpuCores
	}
	if current.CurrentMemoryGi == 0 {
		current.CurrentMemoryGi = fallback.CurrentMemoryGi
	}
	if current.CurrentDiskGb == 0 {
		current.CurrentDiskGb = fallback.CurrentDiskGb
	}
	if current.TargetCpuCores == 0 {
		current.TargetCpuCores = fallback.TargetCpuCores
	}
	if current.TargetMemoryGi == 0 {
		current.TargetMemoryGi = fallback.TargetMemoryGi
	}
	if current.TargetDiskGb == 0 {
		current.TargetDiskGb = fallback.TargetDiskGb
	}
}

func ticketSummaryToItemSummary(summary *generated.TicketSummary) generated.TicketItemSummary {
	if summary == nil {
		return generated.TicketItemSummary{}
	}
	return generated.TicketItemSummary{
		VmId:               summary.VmId,
		VmName:             summary.VmName,
		SystemId:           summary.SystemId,
		SystemName:         summary.SystemName,
		ServiceId:          summary.ServiceId,
		ServiceName:        summary.ServiceName,
		Namespace:          summary.Namespace,
		ClusterId:          summary.ClusterId,
		ClusterName:        summary.ClusterName,
		ClusterEnvironment: summary.ClusterEnvironment,
		OwnerDisplayName:   summary.OwnerDisplayName,
		OwnerUsername:      summary.OwnerUsername,
		TemplateId:         summary.TemplateId,
		TemplateName:       summary.TemplateName,
		InstanceSizeId:     summary.InstanceSizeId,
		InstanceSizeName:   summary.InstanceSizeName,
		RequestVmStatus:    summary.RequestVmStatus,
		LatestVmStatus:     summary.LatestVmStatus,
		CurrentCpuCores:    summary.CurrentCpuCores,
		CurrentMemoryGi:    summary.CurrentMemoryGi,
		CurrentDiskGb:      summary.CurrentDiskGb,
		TargetCpuCores:     summary.TargetCpuCores,
		TargetMemoryGi:     summary.TargetMemoryGi,
		TargetDiskGb:       summary.TargetDiskGb,
		PowerAction:        summary.PowerAction,
	}
}

func approvalActorIdentity(
	actorID string,
	actorByID map[string]approvalActorLookup,
) (displayName, username string) {
	trimmedID := strings.TrimSpace(actorID)
	if trimmedID == "" {
		return "", ""
	}
	if actor, ok := actorByID[trimmedID]; ok {
		displayName = firstNonEmptyString(
			strings.TrimSpace(actor.DisplayName),
			strings.TrimSpace(actor.Username),
			trimmedID,
		)
		username = firstNonEmptyString(
			strings.TrimSpace(actor.Username),
			trimmedID,
		)
		return displayName, username
	}
	return trimmedID, trimmedID
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
