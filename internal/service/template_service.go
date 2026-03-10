package service

import (
	"context"
	"fmt"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/template"
)

// TemplateService handles template business logic (ADR-0007, ADR-0018, ADR-0036).
// Templates define boot source kind and cloud-init only.
// Hardware configuration belongs exclusively to InstanceSize.
type TemplateService struct {
	client *ent.Client
}

// NewTemplateService creates a new TemplateService.
func NewTemplateService(client *ent.Client) *TemplateService {
	return &TemplateService{client: client}
}

// GetActiveTemplate returns the enabled template with the given name.
func (s *TemplateService) GetActiveTemplate(ctx context.Context, name string) (*ent.Template, error) {
	t, err := s.client.Template.Query().
		Where(
			template.NameEQ(name),
			template.EnabledEQ(true),
		).
		First(ctx)
	if err != nil {
		return nil, fmt.Errorf("get active template %s: %w", name, err)
	}
	return t, nil
}

// GetLatestTemplate returns the template with the given name regardless of enabled status.
func (s *TemplateService) GetLatestTemplate(ctx context.Context, name string) (*ent.Template, error) {
	t, err := s.client.Template.Query().
		Where(template.NameEQ(name)).
		First(ctx)
	if err != nil {
		return nil, fmt.Errorf("get latest template %s: %w", name, err)
	}
	return t, nil
}

// ListTemplates returns all enabled templates.
func (s *TemplateService) ListTemplates(ctx context.Context) ([]*ent.Template, error) {
	return s.client.Template.Query().
		Where(template.EnabledEQ(true)).
		Order(ent.Asc(template.FieldName)).
		All(ctx)
}

// GetByID returns a template by ID.
func (s *TemplateService) GetByID(ctx context.Context, id string) (*ent.Template, error) {
	t, err := s.client.Template.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get template %s: %w", id, err)
	}
	return t, nil
}

// CreateTemplate creates a new template.
func (s *TemplateService) CreateTemplate(
	ctx context.Context,
	id, name, createdBy string,
	sourceType, imageURL, pvcName, cloudInit string,
) (*ent.Template, error) {
	create := s.client.Template.Create().
		SetID(id).
		SetName(name).
		SetCreatedBy(createdBy)
	if sourceType != "" {
		create = create.SetSourceType(NormalizeTemplateSourceType(sourceType))
	}
	if imageURL != "" {
		create = create.SetImageURL(imageURL)
	}
	if pvcName != "" {
		create = create.SetPvcName(pvcName)
	}
	if cloudInit != "" {
		create = create.SetCloudInit(cloudInit)
	}
	t, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create template %s: %w", name, err)
	}
	return t, nil
}
