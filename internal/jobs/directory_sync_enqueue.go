package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	entdirectorysyncjob "kv-shepherd.io/shepherd/ent/directorysyncjob"
	"kv-shepherd.io/shepherd/internal/service"
)

// DirectorySyncEnqueueInput captures the persisted directory sync job and the
// River claim-check job that must be committed atomically.
type DirectorySyncEnqueueInput struct {
	JobID              string
	AuthProviderID     string
	RequestSnapshot    map[string]interface{}
	ConflictResolution string
	SyncMode           string
	JoinKeyType        string
	TriggeredBy        string
}

// DirectorySyncEnqueueResult is the committed DirectorySyncJob projection
// needed by callers that return or audit the enqueue operation.
type DirectorySyncEnqueueResult struct {
	JobID       string
	Status      entdirectorysyncjob.Status
	SyncMode    string
	JoinKeyType string
}

// CreateDirectorySyncJobAndEnqueue writes DirectorySyncJob and the River
// claim-check job in one pgx transaction, matching River's InsertTx contract.
func CreateDirectorySyncJobAndEnqueue(
	ctx context.Context,
	pool *pgxpool.Pool,
	riverClient *river.Client[pgx.Tx],
	input DirectorySyncEnqueueInput,
) (*DirectorySyncEnqueueResult, error) {
	if pool == nil || riverClient == nil {
		return nil, fmt.Errorf("directory sync enqueue dependencies are not initialized")
	}

	jobID := strings.TrimSpace(input.JobID)
	authProviderID := strings.TrimSpace(input.AuthProviderID)
	triggeredBy := strings.TrimSpace(input.TriggeredBy)
	if jobID == "" {
		return nil, fmt.Errorf("directory sync job id is required")
	}
	if authProviderID == "" {
		return nil, fmt.Errorf("auth provider id is required")
	}
	if triggeredBy == "" {
		return nil, fmt.Errorf("directory sync triggered_by is required")
	}

	conflictResolution := strings.TrimSpace(input.ConflictResolution)
	if conflictResolution == "" {
		conflictResolution = service.DirectoryConflictResolutionSkip
	}
	syncMode := strings.TrimSpace(input.SyncMode)
	if syncMode == "" {
		syncMode = service.DirectoryExecutionModeManualImport
	}
	joinKeyType := strings.TrimSpace(input.JoinKeyType)

	requestSnapshot, err := json.Marshal(cloneJSONMap(input.RequestSnapshot))
	if err != nil {
		return nil, fmt.Errorf("marshal directory sync request snapshot: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin directory sync enqueue tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
INSERT INTO directory_sync_jobs (
	id,
	created_at,
	updated_at,
	auth_provider_id,
	status,
	request_snapshot,
	conflict_resolution,
	sync_mode,
	join_key_type,
	total_entries,
	create_count,
	update_count,
	blocked_count,
	error_count,
	errors,
	triggered_by
) VALUES (
	$1, $2, $2, $3, $4, $5::jsonb, $6, $7, $8, 0, 0, 0, 0, 0, '[]'::jsonb, $9
)`,
		jobID,
		now,
		authProviderID,
		string(entdirectorysyncjob.StatusPending),
		requestSnapshot,
		conflictResolution,
		syncMode,
		joinKeyType,
		triggeredBy,
	); err != nil {
		return nil, fmt.Errorf("create directory sync job %s: %w", jobID, err)
	}

	if _, err := riverClient.InsertTx(ctx, tx, DirectorySyncArgs{JobID: jobID}, nil); err != nil {
		return nil, fmt.Errorf("enqueue directory sync job %s: %w", jobID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit directory sync enqueue tx: %w", err)
	}

	return &DirectorySyncEnqueueResult{
		JobID:       jobID,
		Status:      entdirectorysyncjob.StatusPending,
		SyncMode:    syncMode,
		JoinKeyType: joinKeyType,
	}, nil
}
