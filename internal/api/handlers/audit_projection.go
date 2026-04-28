package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/authprovider"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entdirectorysyncjob "kv-shepherd.io/shepherd/ent/directorysyncjob"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entinstancesize "kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	"kv-shepherd.io/shepherd/ent/role"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entsystem "kv-shepherd.io/shepherd/ent/system"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entuser "kv-shepherd.io/shepherd/ent/user"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

type auditResourceKey struct {
	ResourceType string
	ResourceID   string
}

func newAuditResourceKey(resourceType, resourceID string) auditResourceKey {
	return auditResourceKey{
		ResourceType: strings.TrimSpace(resourceType),
		ResourceID:   strings.TrimSpace(resourceID),
	}
}

func (s *Server) buildAuditPresentation(
	ctx context.Context,
	logs []*ent.AuditLog,
) (
	actorSummaries map[string]generated.AuditActorSummary,
	resourceSummaries map[auditResourceKey]generated.AuditResourceSummary,
	ticketSummaries map[string]*generated.TicketSummary,
) {
	actorSummaries = s.loadAuditActorSummaries(ctx, logs)
	ticketSummaries = s.loadAuditTicketSummaries(ctx, logs)
	resourceSummaries = s.loadAuditResourceSummaries(ctx, logs, ticketSummaries)
	return actorSummaries, resourceSummaries, ticketSummaries
}

func (s *Server) loadAuditActorSummaries(
	ctx context.Context,
	logs []*ent.AuditLog,
) map[string]generated.AuditActorSummary {
	actorSet := make(map[string]struct{})
	for _, entry := range logs {
		if entry == nil {
			continue
		}
		if actor := strings.TrimSpace(entry.Actor); actor != "" {
			actorSet[actor] = struct{}{}
		}
	}

	summaries := make(map[string]generated.AuditActorSummary, len(actorSet))
	if len(actorSet) == 0 {
		return summaries
	}

	actorIDs := sortedStringSet(actorSet)
	users, err := s.client.User.Query().
		Where(entuser.IDIn(actorIDs...)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to resolve audit actor summaries", zap.Error(err))
	} else {
		for _, user := range users {
			if user == nil {
				continue
			}
			summaries[user.ID] = auditActorSummaryFromUser(user)
		}
	}

	for _, actorID := range actorIDs {
		if _, ok := summaries[actorID]; ok {
			continue
		}
		summaries[actorID] = generated.AuditActorSummary{
			DisplayName: actorID,
		}
	}

	return summaries
}

func auditActorSummaryFromUser(user *ent.User) generated.AuditActorSummary {
	if user == nil {
		return generated.AuditActorSummary{}
	}
	displayName := firstNonEmptyString(
		strings.TrimSpace(user.DisplayName),
		strings.TrimSpace(user.Username),
		strings.TrimSpace(user.Email),
		strings.TrimSpace(user.ID),
	)
	secondaryParts := make([]string, 0, 2)
	if username := strings.TrimSpace(user.Username); username != "" && username != displayName {
		secondaryParts = append(secondaryParts, username)
	}
	if email := strings.TrimSpace(user.Email); email != "" && !slices.Contains(secondaryParts, email) && email != displayName {
		secondaryParts = append(secondaryParts, email)
	}
	return generated.AuditActorSummary{
		DisplayName: displayName,
		Secondary:   strings.Join(secondaryParts, " · "),
	}
}

func (s *Server) loadAuditTicketSummaries(
	ctx context.Context,
	logs []*ent.AuditLog,
) map[string]*generated.TicketSummary {
	ticketIDs := make([]string, 0)
	ticketIDSet := make(map[string]struct{})
	for _, entry := range logs {
		if entry == nil || strings.TrimSpace(entry.ResourceType) != "ticket" {
			continue
		}
		if ticketID := strings.TrimSpace(entry.ResourceID); ticketID != "" {
			if _, exists := ticketIDSet[ticketID]; exists {
				continue
			}
			ticketIDSet[ticketID] = struct{}{}
			ticketIDs = append(ticketIDs, ticketID)
		}
	}
	if len(ticketIDs) == 0 {
		return map[string]*generated.TicketSummary{}
	}

	tickets, err := s.client.Ticket.Query().
		Where(entticket.IDIn(ticketIDs...)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch audit ticket summaries", zap.Error(err))
		return map[string]*generated.TicketSummary{}
	}
	if len(tickets) == 0 {
		return map[string]*generated.TicketSummary{}
	}

	eventIDs := make([]string, 0, len(tickets))
	for _, ticket := range tickets {
		if ticket == nil || strings.TrimSpace(ticket.EventID) == "" {
			continue
		}
		eventIDs = append(eventIDs, ticket.EventID)
	}
	eventPayloadMap := make(map[string][]byte, len(eventIDs))
	if len(eventIDs) > 0 {
		events, eventErr := s.client.DomainEvent.Query().
			Where(domainevent.IDIn(eventIDs...)).
			All(ctx)
		if eventErr != nil {
			logger.Warn("failed to fetch audit ticket events", zap.Error(eventErr))
		} else {
			for _, event := range events {
				if event == nil {
					continue
				}
				eventPayloadMap[event.ID] = event.Payload
			}
		}
	}

	return s.buildAuditTicketSummariesFromTicketsWithEvents(ctx, tickets, eventPayloadMap)
}

func (s *Server) loadAuditResourceSummaries(
	ctx context.Context,
	logs []*ent.AuditLog,
	ticketSummaries map[string]*generated.TicketSummary,
) map[auditResourceKey]generated.AuditResourceSummary {
	byKey := make(map[auditResourceKey]generated.AuditResourceSummary)
	if len(logs) == 0 {
		return byKey
	}

	resourceIDsByType := make(map[string]map[string]struct{})
	for _, entry := range logs {
		if entry == nil {
			continue
		}
		resourceType := strings.TrimSpace(entry.ResourceType)
		resourceID := strings.TrimSpace(entry.ResourceID)
		if resourceType == "" || resourceID == "" {
			continue
		}
		if _, ok := resourceIDsByType[resourceType]; !ok {
			resourceIDsByType[resourceType] = make(map[string]struct{})
		}
		resourceIDsByType[resourceType][resourceID] = struct{}{}
	}

	if ids := sortedStringSet(resourceIDsByType["user"]); len(ids) > 0 {
		users, err := s.client.User.Query().Where(entuser.IDIn(ids...)).All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit user summaries", zap.Error(err))
		} else {
			for _, user := range users {
				if user == nil {
					continue
				}
				actorSummary := auditActorSummaryFromUser(user)
				byKey[newAuditResourceKey("user", user.ID)] = generated.AuditResourceSummary{
					DisplayName: actorSummary.DisplayName,
					Secondary:   actorSummary.Secondary,
				}
			}
		}
	}

	if ids := sortedStringSet(resourceIDsByType["cluster"]); len(ids) > 0 {
		clusters, err := s.client.Cluster.Query().Where(entcluster.IDIn(ids...)).All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit cluster summaries", zap.Error(err))
		} else {
			for _, item := range clusters {
				if item == nil {
					continue
				}
				byKey[newAuditResourceKey("cluster", item.ID)] = generated.AuditResourceSummary{
					DisplayName: firstNonEmptyString(strings.TrimSpace(item.DisplayName), strings.TrimSpace(item.Name), item.ID),
					Secondary:   strings.TrimSpace(string(item.Environment)),
				}
			}
		}
	}

	if ids := sortedStringSet(resourceIDsByType["namespace"]); len(ids) > 0 {
		namespaces, err := s.client.NamespaceRegistry.Query().Where(namespaceregistry.IDIn(ids...)).All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit namespace summaries", zap.Error(err))
		} else {
			for _, item := range namespaces {
				if item == nil {
					continue
				}
				byKey[newAuditResourceKey("namespace", item.ID)] = generated.AuditResourceSummary{
					DisplayName: firstNonEmptyString(strings.TrimSpace(item.Name), item.ID),
					Secondary:   strings.TrimSpace(string(item.Environment)),
				}
			}
		}
	}

	if ids := sortedStringSet(resourceIDsByType["system"]); len(ids) > 0 {
		systems, err := s.client.System.Query().Where(entsystem.IDIn(ids...)).All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit system summaries", zap.Error(err))
		} else {
			for _, item := range systems {
				if item == nil {
					continue
				}
				byKey[newAuditResourceKey("system", item.ID)] = generated.AuditResourceSummary{
					DisplayName: firstNonEmptyString(strings.TrimSpace(item.Name), item.ID),
					Secondary:   strings.TrimSpace(item.Description),
				}
			}
		}
	}

	if ids := sortedStringSet(resourceIDsByType["service"]); len(ids) > 0 {
		services, err := s.client.Service.Query().
			Where(entservice.IDIn(ids...)).
			WithSystem(func(query *ent.SystemQuery) {
				query.Select(entsystem.FieldID, entsystem.FieldName)
			}).
			All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit service summaries", zap.Error(err))
		} else {
			for _, item := range services {
				if item == nil {
					continue
				}
				summary := generated.AuditResourceSummary{
					DisplayName: firstNonEmptyString(strings.TrimSpace(item.Name), item.ID),
				}
				if item.Edges.System != nil {
					summary.Secondary = strings.TrimSpace(item.Edges.System.Name)
				}
				byKey[newAuditResourceKey("service", item.ID)] = summary
			}
		}
	}

	vmClusterNames := s.loadAuditVMClusterNames(ctx, resourceIDsByType["vm"])
	if ids := sortedStringSet(resourceIDsByType["vm"]); len(ids) > 0 {
		vms, err := s.client.VM.Query().
			Where(entvm.IDIn(ids...)).
			WithService(func(query *ent.ServiceQuery) {
				query.Select(entservice.FieldID, entservice.FieldName).
					WithSystem(func(systemQuery *ent.SystemQuery) {
						systemQuery.Select(entsystem.FieldID, entsystem.FieldName)
					})
			}).
			All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit vm summaries", zap.Error(err))
		} else {
			for _, item := range vms {
				if item == nil {
					continue
				}
				summary := generated.AuditResourceSummary{
					DisplayName: firstNonEmptyString(strings.TrimSpace(item.Name), item.ID),
				}
				scope := make([]string, 0, 2)
				if item.Edges.Service != nil {
					if item.Edges.Service.Edges.System != nil && strings.TrimSpace(item.Edges.Service.Edges.System.Name) != "" {
						scope = append(scope, strings.TrimSpace(item.Edges.Service.Edges.System.Name))
					}
					if strings.TrimSpace(item.Edges.Service.Name) != "" {
						scope = append(scope, strings.TrimSpace(item.Edges.Service.Name))
					}
				}
				summary.Secondary = strings.Join(scope, " / ")
				summary.Tertiary = joinAuditSummaryParts(
					strings.TrimSpace(item.Namespace),
					vmClusterNames[strings.TrimSpace(item.ClusterID)],
				)
				byKey[newAuditResourceKey("vm", item.ID)] = summary
			}
		}
	}

	if ids := sortedStringSet(resourceIDsByType["template"]); len(ids) > 0 {
		templates, err := s.client.Template.Query().Where(enttemplate.IDIn(ids...)).All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit template summaries", zap.Error(err))
		} else {
			for _, item := range templates {
				if item == nil {
					continue
				}
				byKey[newAuditResourceKey("template", item.ID)] = generated.AuditResourceSummary{
					DisplayName: firstNonEmptyString(strings.TrimSpace(item.DisplayName), strings.TrimSpace(item.Name), item.ID),
					Secondary:   joinAuditSummaryParts(strings.TrimSpace(item.OsFamily), strings.TrimSpace(item.OsVersion)),
				}
			}
		}
	}

	if ids := sortedStringSet(resourceIDsByType["instance_size"]); len(ids) > 0 {
		sizes, err := s.client.InstanceSize.Query().Where(entinstancesize.IDIn(ids...)).All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit instance size summaries", zap.Error(err))
		} else {
			for _, item := range sizes {
				if item == nil {
					continue
				}
				byKey[newAuditResourceKey("instance_size", item.ID)] = generated.AuditResourceSummary{
					DisplayName: firstNonEmptyString(strings.TrimSpace(item.DisplayName), strings.TrimSpace(item.Name), item.ID),
					Secondary:   summarizeAuditInstanceSize(item),
				}
			}
		}
	}

	if ids := sortedStringSet(resourceIDsByType["role"]); len(ids) > 0 {
		roles, err := s.client.Role.Query().Where(role.IDIn(ids...)).All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit role summaries", zap.Error(err))
		} else {
			for _, item := range roles {
				if item == nil {
					continue
				}
				secondary := ""
				if item.BuiltIn {
					secondary = "built-in"
				}
				byKey[newAuditResourceKey("role", item.ID)] = generated.AuditResourceSummary{
					DisplayName: firstNonEmptyString(strings.TrimSpace(item.Name), item.ID),
					Secondary:   secondary,
				}
			}
		}
	}

	if ids := sortedStringSet(resourceIDsByType["auth_provider"]); len(ids) > 0 {
		providers, err := s.client.AuthProvider.Query().Where(authprovider.IDIn(ids...)).All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit auth provider summaries", zap.Error(err))
		} else {
			for _, item := range providers {
				if item == nil {
					continue
				}
				byKey[newAuditResourceKey("auth_provider", item.ID)] = generated.AuditResourceSummary{
					DisplayName: firstNonEmptyString(strings.TrimSpace(item.Name), item.ID),
					Secondary:   strings.TrimSpace(item.AuthType),
				}
			}
		}
	}

	if ids := sortedStringSet(resourceIDsByType["directory_sync_job"]); len(ids) > 0 {
		jobs, err := s.client.DirectorySyncJob.Query().
			Where(entdirectorysyncjob.IDIn(ids...)).
			All(ctx)
		if err != nil {
			logger.Warn("failed to fetch audit directory sync job summaries", zap.Error(err))
		} else {
			providerIDSet := make(map[string]struct{})
			for _, job := range jobs {
				if job == nil || strings.TrimSpace(job.AuthProviderID) == "" {
					continue
				}
				providerIDSet[job.AuthProviderID] = struct{}{}
			}
			providerNamesByID := make(map[string]string, len(providerIDSet))
			if len(providerIDSet) > 0 {
				providers, providerErr := s.client.AuthProvider.Query().
					Where(authprovider.IDIn(sortedStringSet(providerIDSet)...)).
					All(ctx)
				if providerErr != nil {
					logger.Warn("failed to fetch auth providers for directory sync summaries", zap.Error(providerErr))
				} else {
					for _, provider := range providers {
						if provider == nil {
							continue
						}
						providerNamesByID[provider.ID] = firstNonEmptyString(
							strings.TrimSpace(provider.Name),
							strings.TrimSpace(provider.ID),
						)
					}
				}
			}
			for _, job := range jobs {
				if job == nil {
					continue
				}
				byKey[newAuditResourceKey("directory_sync_job", job.ID)] = generated.AuditResourceSummary{
					DisplayName: firstNonEmptyString(
						providerNamesByID[job.AuthProviderID],
						strings.TrimSpace(job.AuthProviderID),
						strings.TrimSpace(job.ID),
					),
					Secondary: strings.TrimSpace(string(job.SyncMode)),
					Tertiary:  strings.TrimSpace(string(job.Status)),
				}
			}
		}
	}

	for ticketID, summary := range ticketSummaries {
		if summary == nil {
			continue
		}
		byKey[newAuditResourceKey("ticket", ticketID)] = summarizeAuditTicket(ticketID, summary)
	}

	for _, entry := range logs {
		if entry == nil {
			continue
		}
		key := newAuditResourceKey(entry.ResourceType, entry.ResourceID)
		if key.ResourceType == "" || key.ResourceID == "" {
			continue
		}
		if _, ok := byKey[key]; ok {
			continue
		}
		byKey[key] = summarizeAuditResourceFromDetails(entry)
	}

	return byKey
}

func (s *Server) loadAuditAuthProviderDisplays(
	ctx context.Context,
	logs []*ent.AuditLog,
) map[string]string {
	byID := make(map[string]string)
	if len(logs) == 0 {
		return byID
	}

	providerIDSet := make(map[string]struct{})
	for _, entry := range logs {
		if entry == nil {
			continue
		}
		if providerID := auditStringField(entry.Details, "auth_provider_id"); providerID != "" {
			providerIDSet[providerID] = struct{}{}
		}
	}
	if len(providerIDSet) == 0 {
		return byID
	}

	providers, err := s.client.AuthProvider.Query().
		Where(authprovider.IDIn(sortedStringSet(providerIDSet)...)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch auth providers for audit message params", zap.Error(err))
		return byID
	}

	for _, provider := range providers {
		if provider == nil {
			continue
		}
		byID[provider.ID] = firstNonEmptyString(
			strings.TrimSpace(provider.Name),
			strings.TrimSpace(provider.ID),
		)
	}
	return byID
}

func (s *Server) loadAuditVMClusterNames(
	ctx context.Context,
	vmIDs map[string]struct{},
) map[string]string {
	clusterNames := make(map[string]string)
	if len(vmIDs) == 0 {
		return clusterNames
	}

	vms, err := s.client.VM.Query().
		Where(entvm.IDIn(sortedStringSet(vmIDs)...)).
		Select(entvm.FieldID, entvm.FieldClusterID).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch vm cluster ids for audit summaries", zap.Error(err))
		return clusterNames
	}

	clusterIDSet := make(map[string]struct{})
	for _, item := range vms {
		if item == nil {
			continue
		}
		if clusterID := strings.TrimSpace(item.ClusterID); clusterID != "" {
			clusterIDSet[clusterID] = struct{}{}
		}
	}
	if len(clusterIDSet) == 0 {
		return clusterNames
	}

	clusters, err := s.client.Cluster.Query().
		Where(entcluster.IDIn(sortedStringSet(clusterIDSet)...)).
		All(ctx)
	if err != nil {
		logger.Warn("failed to fetch vm clusters for audit summaries", zap.Error(err))
		return clusterNames
	}
	for _, item := range clusters {
		if item == nil {
			continue
		}
		clusterNames[item.ID] = firstNonEmptyString(strings.TrimSpace(item.DisplayName), strings.TrimSpace(item.Name), item.ID)
	}
	return clusterNames
}

func (s *Server) buildAuditTicketSummariesFromTicketsWithEvents(
	ctx context.Context,
	tickets []*ent.Ticket,
	eventPayloadMap map[string][]byte,
) map[string]*generated.TicketSummary {
	summaries := make(map[string]*generated.TicketSummary, len(tickets))
	if len(tickets) == 0 {
		return summaries
	}

	templateIDs, instanceSizeIDs := collectApprovalCatalogLookupIDs(eventPayloadMap)
	serviceByID := s.loadApprovalServiceLookups(
		ctx,
		collectApprovalPrefillServiceIDs(eventPayloadMap),
	)
	vmByID, vmTemplateIDs, vmInstanceSizeIDs := s.loadApprovalVMContexts(
		ctx,
		collectApprovalSummaryVMIDs(eventPayloadMap),
		serviceByID,
	)
	templateIDs = append(templateIDs, vmTemplateIDs...)
	instanceSizeIDs = append(instanceSizeIDs, vmInstanceSizeIDs...)
	templateByID, instanceSizeByID := s.loadApprovalCatalogLookups(
		ctx,
		sortedStringSet(sliceToStringSet(templateIDs)),
		sortedStringSet(sliceToStringSet(instanceSizeIDs)),
	)
	actorLookupByID := s.loadApprovalActorLookups(ctx, tickets)
	batchFallbackItemsByParentID := s.loadApprovalBatchChildFallbackItems(
		ctx,
		tickets,
		templateByID,
		instanceSizeByID,
		serviceByID,
		vmByID,
		actorLookupByID,
	)

	for _, ticket := range tickets {
		if ticket == nil {
			continue
		}
		var payload map[string]interface{}
		if raw := eventPayloadMap[ticket.EventID]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &payload); err != nil {
				logger.Warn("failed to decode audit ticket event payload",
					zap.String("ticket_id", ticket.ID),
					zap.String("event_id", ticket.EventID),
					zap.Error(err),
				)
			}
		}
		if payload == nil {
			payload = map[string]interface{}{}
		}
		summary := buildTicketSummary(
			ticket,
			payload,
			templateByID,
			instanceSizeByID,
			serviceByID,
			vmByID,
			actorLookupByID,
			batchFallbackItemsByParentID[ticket.ID],
		)
		if summary != nil {
			if requesterID := strings.TrimSpace(ticket.Requester); requesterID != "" {
				if requester, ok := actorLookupByID[requesterID]; ok {
					summary.RequesterDisplayName = firstNonEmptyString(
						strings.TrimSpace(requester.DisplayName),
						strings.TrimSpace(requester.Username),
						requesterID,
					)
					summary.RequesterUsername = firstNonEmptyString(
						strings.TrimSpace(requester.Username),
						requesterID,
					)
				} else {
					summary.RequesterDisplayName = requesterID
					summary.RequesterUsername = requesterID
				}
			}
			if approverID := strings.TrimSpace(ticket.Approver); approverID != "" {
				if approver, ok := actorLookupByID[approverID]; ok {
					summary.ApproverDisplayName = firstNonEmptyString(
						strings.TrimSpace(approver.DisplayName),
						strings.TrimSpace(approver.Username),
						approverID,
					)
					summary.ApproverUsername = firstNonEmptyString(
						strings.TrimSpace(approver.Username),
						approverID,
					)
				} else {
					summary.ApproverDisplayName = approverID
					summary.ApproverUsername = approverID
				}
			}
		}
		summaries[ticket.ID] = summary
	}

	return summaries
}

func summarizeAuditTicket(
	ticketID string,
	summary *generated.TicketSummary,
) generated.AuditResourceSummary {
	if summary == nil {
		return generated.AuditResourceSummary{DisplayName: ticketID}
	}
	displayName := firstNonEmptyString(
		strings.TrimSpace(summary.VmName),
		strings.TrimSpace(summary.ServiceName),
		strings.TrimSpace(summary.SystemName),
		strings.TrimSpace(summary.TemplateName),
		strings.TrimSpace(summary.InstanceSizeName),
		strings.TrimSpace(ticketID),
	)
	secondary := joinAuditSummaryParts(
		strings.TrimSpace(summary.SystemName),
		strings.TrimSpace(summary.ServiceName),
		strings.TrimSpace(summary.Namespace),
		firstNonEmptyString(strings.TrimSpace(summary.ClusterName), strings.TrimSpace(summary.ClusterId)),
	)
	tertiary := joinAuditSummaryParts(
		strings.TrimSpace(summary.TemplateName),
		strings.TrimSpace(summary.InstanceSizeName),
		summarizeAuditResourceTargets(summary.TargetCpuCores, summary.TargetMemoryGi, summary.TargetDiskGb),
	)
	return generated.AuditResourceSummary{
		DisplayName: displayName,
		Secondary:   secondary,
		Tertiary:    tertiary,
	}
}

func summarizeAuditInstanceSize(item *ent.InstanceSize) string {
	if item == nil {
		return ""
	}
	return summarizeAuditResourceTargets(item.CPUCores, item.MemoryGi, item.DiskGB)
}

func summarizeAuditResourceTargets(cpu, memoryGi float64, diskGb int) string {
	parts := make([]string, 0, 3)
	if cpu > 0 {
		parts = append(parts, trimAuditFloat(cpu)+" vCPU")
	}
	if memoryGi > 0 {
		parts = append(parts, trimAuditFloat(memoryGi)+" Gi")
	}
	if diskGb > 0 {
		parts = append(parts, fmt.Sprintf("%d Gi", diskGb))
	}
	return strings.Join(parts, " · ")
}

func trimAuditFloat(value float64) string {
	if value == float64(int64(value)) {
		return fmt.Sprintf("%d", int64(value))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
}

func summarizeAuditResourceFromDetails(entry *ent.AuditLog) generated.AuditResourceSummary {
	if entry == nil {
		return generated.AuditResourceSummary{}
	}
	displayName := firstNonEmptyString(
		auditStringField(entry.Details, "display_name"),
		auditStringField(entry.Details, "name"),
		auditStringField(entry.Details, "vm_name"),
		auditStringField(entry.Details, "service_name"),
		auditStringField(entry.Details, "system_name"),
		auditStringField(entry.Details, "provider_name"),
		strings.TrimSpace(entry.ResourceID),
	)
	secondary := joinAuditSummaryParts(
		auditStringField(entry.Details, "system_name"),
		auditStringField(entry.Details, "service_name"),
		auditStringField(entry.Details, "namespace"),
		firstNonEmptyString(
			auditStringField(entry.Details, "cluster_name"),
			auditStringField(entry.Details, "selected_cluster_name"),
		),
	)
	tertiary := joinAuditSummaryParts(
		auditStringField(entry.Details, "auth_type"),
		auditStringField(entry.Details, "mode"),
		auditStringField(entry.Details, "join_key"),
	)
	return generated.AuditResourceSummary{
		DisplayName: displayName,
		Secondary:   secondary,
		Tertiary:    tertiary,
	}
}

func joinAuditSummaryParts(parts ...string) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || slices.Contains(values, trimmed) {
			continue
		}
		values = append(values, trimmed)
	}
	return strings.Join(values, " · ")
}
