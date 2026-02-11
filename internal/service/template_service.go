package service

import (
	"context"
	"fmt"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/template"
)

// TemplateService handles template business logic (ADR-0007, ADR-0018).
// Templates define OS image source and cloud-init only.
// No Go Template variables (removed per ADR-0018).
type TemplateService struct {
	client *ent.Client
}

// NewTemplateService creates a new TemplateService.
func NewTemplateService(client *ent.Client) *TemplateService {
	return &TemplateService{client: client}
}

// GetActiveTemplate returns the latest active version of a template by name.
func (s *TemplateService) GetActiveTemplate(ctx context.Context, name string) (*ent.Template, error) {
	t, err := s.client.Template.Query().
		Where(
			template.NameEQ(name),
			template.EnabledEQ(true),
		).
		Order(ent.Desc(template.FieldVersion)).
		First(ctx)
	if err != nil {
		return nil, fmt.Errorf("get active template %s: %w", name, err)
	}
	return t, nil
}

// GetLatestTemplate returns the latest version of a template (active or not).
func (s *TemplateService) GetLatestTemplate(ctx context.Context, name string) (*ent.Template, error) {
	t, err := s.client.Template.Query().
		Where(template.NameEQ(name)).
		Order(ent.Desc(template.FieldVersion)).
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
		Order(ent.Asc(template.FieldName), ent.Desc(template.FieldVersion)).
		All(ctx)
}

// CreateTemplate creates a new template version.
func (s *TemplateService) CreateTemplate(ctx context.Context, id, name, createdBy string, version int, spec map[string]interface{}) (*ent.Template, error) {
	t, err := s.client.Template.Create().
		SetID(id).
		SetName(name).
		SetVersion(version).
		SetSpec(spec).
		SetCreatedBy(createdBy).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create template %s v%d: %w", name, version, err)
	}
	return t, nil
}
