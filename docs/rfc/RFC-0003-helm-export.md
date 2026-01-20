# RFC-0003: Helm Chart Export

> **Status**: Deferred  
> **Priority**: P2  
> **Source**: ADR-0007  
> **Trigger**: Users need to export templates as standard Helm Charts

---

## Problem

Users may need to export platform-managed VM templates as portable Helm Charts for use outside the governance platform.

---

## Current State

**Not implementing**

Templates are stored in PostgreSQL and used within the platform. Export functionality is not a V1.0 priority.

---

## Proposed Solution

```go
// internal/service/helm_export_service.go

type HelmExportService struct {
    templateRepo *repository.TemplateRepository
}

// Export generates a Helm Chart from template
func (s *HelmExportService) Export(ctx context.Context, templateID int) (*HelmChart, error) {
    tmpl, _ := s.templateRepo.Get(ctx, templateID)
    
    return &HelmChart{
        Name:        tmpl.Name,
        Version:     fmt.Sprintf("0.1.%d", tmpl.Version),
        AppVersion:  "1.0.0",
        Templates:   []string{tmpl.Content},
        Values:      extractDefaultValues(tmpl.Content),
    }, nil
}
```

---

## API Endpoint

```
GET /api/v1/templates/{id}/export/helm
Accept: application/gzip

Response: Helm Chart archive (.tgz)
```

---

## References

- [Helm Chart Structure](https://helm.sh/docs/topics/charts/)
- [ADR-0007: Template Storage](../adr/ADR-0007-template-storage.md)
