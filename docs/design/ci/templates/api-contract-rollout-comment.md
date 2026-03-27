<!-- Use this template for issue/PR rollout comments about API contract gates. -->

## API Contract Rollout Note

### Terminology Compatibility

- `make api-diff` == `make api-breaking` (alias kept for historical Issue wording compatibility).
- Canonical spec: `api/openapi.yaml`.
- Compat artifact: `api/openapi.compat.yaml` (derived, Go codegen-only).

### Workflow/Gate Location

- Current workflow file: `.github/workflows/api-contract-validation.yml`.
- Historical references to `api-check.yml` or older gate names should be mapped to this workflow.

### Required Local Verification

```bash
make api-compat-generate
make api-generate
REQUIRE_OPENAPI_COMPAT=1 make api-compat
REQUIRE_OPENAPI_COMPAT=1 make api-check
make api-lint
make api-breaking
make api-contract-test
```

### Upstream Tracking

- `oapi-codegen` OpenAPI 3.1 support tracking issue: https://github.com/oapi-codegen/oapi-codegen/issues/373
