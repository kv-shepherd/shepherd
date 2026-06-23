package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	entpendingadoption "kv-shepherd.io/shepherd/ent/pendingadoption"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	infracontract "kv-shepherd.io/shepherd/internal/provider/infracontract"
)

const pendingAdoptionVMResourceType = "VirtualMachine"

type AdoptionDiscoveryService struct {
	client    *ent.Client
	vmService *VMService
}

type AdoptionDiscoveryInput struct {
	ClusterID    string
	Namespace    string
	DiscoveredBy string
}

type AdoptionDiscoveryResult struct {
	Scanned                int
	Created                int
	Refreshed              int
	SkippedInvalid         int
	SkippedExistingVM      int
	SkippedMissingService  int
	SkippedAlreadyResolved int
}

func NewAdoptionDiscoveryService(client *ent.Client, vmService *VMService) *AdoptionDiscoveryService {
	return &AdoptionDiscoveryService{client: client, vmService: vmService}
}

func (s *AdoptionDiscoveryService) DiscoverVMs(ctx context.Context, input AdoptionDiscoveryInput) (*AdoptionDiscoveryResult, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("adoption discovery requires ent client")
	}
	if s.vmService == nil {
		return nil, fmt.Errorf("adoption discovery requires vm service")
	}

	clusterID := strings.TrimSpace(input.ClusterID)
	namespace := strings.TrimSpace(input.Namespace)
	if clusterID == "" {
		return nil, fmt.Errorf("adoption discovery requires cluster id")
	}
	if namespace == "" {
		return nil, fmt.Errorf("adoption discovery requires namespace")
	}

	list, err := s.vmService.ListVMs(ctx, clusterID, namespace, infracontract.ListOptions{
		LabelSelector: domain.ShepherdServiceIDLabel,
	})
	if err != nil {
		return nil, fmt.Errorf("list labeled k8s vms for adoption discovery: %w", err)
	}

	result := &AdoptionDiscoveryResult{}
	if list == nil {
		return result, nil
	}

	for _, candidate := range list.Items {
		result.Scanned++
		action, err := s.discoverCandidate(ctx, input, candidate)
		if err != nil {
			return nil, err
		}
		switch action {
		case adoptionDiscoveryCreated:
			result.Created++
		case adoptionDiscoveryRefreshed:
			result.Refreshed++
		case adoptionDiscoverySkippedExistingVM:
			result.SkippedExistingVM++
		case adoptionDiscoverySkippedMissingService:
			result.SkippedMissingService++
		case adoptionDiscoverySkippedAlreadyResolved:
			result.SkippedAlreadyResolved++
		case adoptionDiscoverySkippedInvalid:
			result.SkippedInvalid++
		}
	}

	return result, nil
}

type adoptionDiscoveryAction int

const (
	adoptionDiscoverySkippedInvalid adoptionDiscoveryAction = iota
	adoptionDiscoveryCreated
	adoptionDiscoveryRefreshed
	adoptionDiscoverySkippedExistingVM
	adoptionDiscoverySkippedMissingService
	adoptionDiscoverySkippedAlreadyResolved
)

func (s *AdoptionDiscoveryService) discoverCandidate(
	ctx context.Context,
	input AdoptionDiscoveryInput,
	candidate *domain.VM,
) (adoptionDiscoveryAction, error) {
	if candidate == nil {
		return adoptionDiscoverySkippedInvalid, nil
	}

	name := strings.TrimSpace(candidate.Name)
	namespace := strings.TrimSpace(candidate.Namespace)
	if namespace == "" {
		namespace = strings.TrimSpace(input.Namespace)
	}
	if name == "" || namespace == "" {
		return adoptionDiscoverySkippedInvalid, nil
	}

	labels := cloneStringMap(candidate.Spec.Labels)
	serviceID := strings.TrimSpace(labels[domain.ShepherdServiceIDLabel])
	if serviceID == "" {
		return adoptionDiscoverySkippedInvalid, nil
	}

	exists, err := s.client.VM.Query().
		Where(entvm.NamespaceEQ(namespace), entvm.NameEQ(name)).
		Exist(ctx)
	if err != nil {
		return adoptionDiscoverySkippedInvalid, fmt.Errorf("check existing vm row for %s/%s: %w", namespace, name, err)
	}
	if exists {
		return adoptionDiscoverySkippedExistingVM, nil
	}

	serviceExists, err := s.client.Service.Query().
		Where(entservice.IDEQ(serviceID)).
		Exist(ctx)
	if err != nil {
		return adoptionDiscoverySkippedInvalid, fmt.Errorf("check adoption service %s for %s/%s: %w", serviceID, namespace, name, err)
	}
	if !serviceExists {
		return adoptionDiscoverySkippedMissingService, nil
	}

	action, err := s.upsertPendingAdoption(ctx, input, namespace, name, labels)
	if err != nil {
		return adoptionDiscoverySkippedInvalid, err
	}
	return action, nil
}

func (s *AdoptionDiscoveryService) upsertPendingAdoption(
	ctx context.Context,
	input AdoptionDiscoveryInput,
	namespace, name string,
	labels map[string]string,
) (adoptionDiscoveryAction, error) {
	clusterID := strings.TrimSpace(input.ClusterID)
	discoveredBy := strings.TrimSpace(input.DiscoveredBy)

	existing, err := s.client.PendingAdoption.Query().
		Where(
			entpendingadoption.ClusterIDEQ(clusterID),
			entpendingadoption.NamespaceEQ(namespace),
			entpendingadoption.ResourceNameEQ(name),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return adoptionDiscoverySkippedInvalid, fmt.Errorf("query pending adoption %s/%s on %s: %w", namespace, name, clusterID, err)
	}
	if err == nil {
		if existing.Status != entpendingadoption.StatusPENDING {
			return adoptionDiscoverySkippedAlreadyResolved, nil
		}
		update := s.client.PendingAdoption.UpdateOneID(existing.ID).
			Where(entpendingadoption.StatusEQ(entpendingadoption.StatusPENDING)).
			SetResourceType(pendingAdoptionVMResourceType).
			SetLabels(labels)
		if discoveredBy != "" {
			update.SetDiscoveredBy(discoveredBy)
		} else {
			update.ClearDiscoveredBy()
		}
		if _, saveErr := update.Save(ctx); saveErr != nil {
			if ent.IsNotFound(saveErr) {
				return adoptionDiscoverySkippedAlreadyResolved, nil
			}
			return adoptionDiscoverySkippedInvalid, fmt.Errorf("refresh pending adoption %s/%s on %s: %w", namespace, name, clusterID, saveErr)
		}
		return adoptionDiscoveryRefreshed, nil
	}

	id, err := newPendingAdoptionID()
	if err != nil {
		return adoptionDiscoverySkippedInvalid, err
	}
	create := s.client.PendingAdoption.Create().
		SetID(id).
		SetClusterID(clusterID).
		SetNamespace(namespace).
		SetResourceName(name).
		SetResourceType(pendingAdoptionVMResourceType).
		SetStatus(entpendingadoption.StatusPENDING).
		SetLabels(labels)
	if discoveredBy != "" {
		create.SetDiscoveredBy(discoveredBy)
	}
	if _, err := create.Save(ctx); err != nil {
		if ent.IsConstraintError(err) {
			return s.upsertPendingAdoption(ctx, input, namespace, name, labels)
		}
		return adoptionDiscoverySkippedInvalid, fmt.Errorf("create pending adoption %s/%s on %s: %w", namespace, name, clusterID, err)
	}
	return adoptionDiscoveryCreated, nil
}

func newPendingAdoptionID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate pending adoption id: %w", err)
	}
	return id.String(), nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
