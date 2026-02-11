// Package middleware - OpenAPI request/response validation middleware.
//
// ADR-0029: Uses libopenapi-validator with StrictMode.
// This file is a Phase 0 placeholder. Full implementation in Phase 1
// after OpenAPI spec is complete.
//
// Phase 1 implementation will:
// 1. Load OpenAPI spec from embedded file
// 2. Validate incoming requests against spec (parameters, body, content-type)
// 3. Validate outgoing responses against spec (status codes, body schema)
// 4. StrictMode: reject unknown fields in request bodies
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/api/middleware
package middleware
