package handlers

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/pkg/schema"
)

// schemaVersion is the embedded schema baseline version.
// Reflects the KubeVirt source release this schema was extracted from.
// Format: semantic version aligned with KubeVirt release tag.
// Increment the patch when the extraction/pruning logic changes without
// a KubeVirt upgrade; bump minor/major when KubeVirt version changes.
const schemaVersion = "1.7.0"

// ── Parse-once cache ──────────────────────────────────────────────────────────
//
// The embedded instancesize.schema.json is 229 KB. Parsing it on every request
// wastes CPU. We parse once on first request per entity type using sync.Once,
// and serve the pre-parsed result directly.
//
// Thread-safety: sync.Once guarantees that only one goroutine runs the
// initialiser; subsequent callers block until it completes and then read
// the cached value. The cached values are never mutated after init.

type schemaCache struct {
	once     sync.Once
	schema   map[string]interface{}
	mask     generated.SchemaMask
	err      error
	cachedAt time.Time // set once during sync.Once initialisation — "schema was cached at"
}

// schemaCaches maps entity_type → lazy-initialised parse cache.
// Add a new entry here when a new entity type is supported.
var schemaCaches = map[string]*schemaCache{
	"instancesize": {},
}

// loadSchemaCache initialises the cache for entityType on first call.
// Must be called with a valid entityType (one that exists in schemaCaches).
func loadSchemaCache(c *schemaCache, entityType string) {
	c.once.Do(func() {
		schemaBytes, _ := schema.SchemaFor(entityType)
		schemaData, err := jsonParseSchema(schemaBytes)
		if err != nil {
			c.err = err
			return
		}
		c.schema = schemaData

		maskBytes, _ := schema.MaskFor(entityType)
		if err := jsonParseMask(maskBytes, &c.mask); err != nil {
			c.err = err
			return
		}

		// Ensure quick_fields is never nil in the response (OpenAPI requires array).
		if c.mask.QuickFields == nil {
			c.mask.QuickFields = []generated.MaskField{}
		}

		// Record when this schema was first parsed and cached.
		// ADR-0023: fetched_at represents cache initialisation time, not request time.
		// Frontend uses this to detect schema drift (cache age > threshold).
		c.cachedAt = time.Now().UTC()
	})
}

// GetDynamicSchema handles GET /schemas/{entity_type}.
//
// Returns the embedded JSON Schema and UI mask for the requested entity type.
// Per ADR-0023, the current implementation serves from the embedded baseline
// (source: "embedded"). When a remote schema cache is available, handlers
// will prefer the cached version and degrade to embedded on fetch errors.
//
// entity_type must be one of: instancesize.
//   - template: excluded — cloud_init is a static YAML textarea (master-flow Step 3).
//   - cluster:  excluded — schema not yet designed (ADR-0023 phase 2).
//
// Returns 400 UNSUPPORTED_ENTITY_TYPE if entity_type is not in the supported set.
//
// ADR-0023: embedded schema is the authoritative baseline; degraded=false
// because the embedded schema IS the current source of truth (no remote
// schema has been configured yet).
func (s *Server) GetDynamicSchema(c *gin.Context, entityType generated.GetDynamicSchemaParamsEntityType) {
	entityTypeStr := string(entityType)

	cache, known := schemaCaches[entityTypeStr]
	if !known {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "UNSUPPORTED_ENTITY_TYPE",
			Message: "unsupported entity_type: " + entityTypeStr,
		})
		return
	}

	// Parse-once: initialises on first request, returns cached result thereafter.
	loadSchemaCache(cache, entityTypeStr)
	if cache.err != nil {
		logger.Error("failed to parse embedded schema/mask",
			zap.String("entity_type", entityTypeStr),
			zap.Error(cache.err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, generated.DynamicSchemaResponse{
		Schema:        cache.schema,
		Mask:          cache.mask,
		SchemaVersion: schemaVersion,
		Source:        generated.Embedded,
		// degraded=false: embedded schema IS the current source of truth.
		// Set to true only when falling back from a more-recent remote schema.
		Degraded: false,
		// FetchedAt records when the schema was first parsed into the in-process cache,
		// NOT the current request time. ADR-0023: frontend uses this for drift detection.
		FetchedAt: cache.cachedAt,
	})
}
