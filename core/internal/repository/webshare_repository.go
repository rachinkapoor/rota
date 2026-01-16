package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alpkeskin/rota/core/internal/database"
	"github.com/alpkeskin/rota/core/internal/models"
	"github.com/jackc/pgx/v5"
)

// WebshareRepository handles webshare sync status database operations
type WebshareRepository struct {
	db *database.DB
}

// NewWebshareRepository creates a new WebshareRepository
func NewWebshareRepository(db *database.DB) *WebshareRepository {
	return &WebshareRepository{db: db}
}

// CreateSyncStatus creates a new sync status record
func (r *WebshareRepository) CreateSyncStatus(ctx context.Context, syncedAt time.Time) (*models.WebshareSyncStatus, error) {
	query := `
		INSERT INTO webshare_sync_status (synced_at, status, created_at, updated_at)
		VALUES ($1, 'IN-PROGRESS', NOW(), NOW())
		RETURNING id, synced_at, status, error, logs, ip_removed, ip_added, ip_replaced, created_at, updated_at
	`

	var status models.WebshareSyncStatus
	err := r.db.Pool.QueryRow(ctx, query, syncedAt).Scan(
		&status.ID,
		&status.SyncedAt,
		&status.Status,
		&status.Error,
		&status.Logs,
		&status.IPRemoved,
		&status.IPAdded,
		&status.IPReplaced,
		&status.CreatedAt,
		&status.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create sync status: %w", err)
	}

	return &status, nil
}

// UpdateSyncStatus updates a sync status record
func (r *WebshareRepository) UpdateSyncStatus(
	ctx context.Context,
	id int,
	status string,
	errorJSON *string,
	logsJSON *string,
	ipRemoved *string,
	ipAdded *string,
	ipReplaced *string,
) error {
	query := `
		UPDATE webshare_sync_status
		SET status = $1,
		    error = $2,
		    logs = $3,
		    ip_removed = $4,
		    ip_added = $5,
		    ip_replaced = $6,
		    updated_at = NOW()
		WHERE id = $7
	`

	_, err := r.db.Pool.Exec(ctx, query, status, errorJSON, logsJSON, ipRemoved, ipAdded, ipReplaced, id)
	if err != nil {
		return fmt.Errorf("failed to update sync status: %w", err)
	}

	return nil
}

// GetLatestSync gets the most recent completed sync (SUCCESS or FAILED)
func (r *WebshareRepository) GetLatestSync(ctx context.Context) (*models.WebshareSyncStatus, error) {
	query := `
		SELECT id, synced_at, status, error, logs, ip_removed, ip_added, ip_replaced, created_at, updated_at
		FROM webshare_sync_status
		WHERE status IN ('SUCCESS', 'FAILED')
		ORDER BY synced_at DESC
		LIMIT 1
	`

	var status models.WebshareSyncStatus
	err := r.db.Pool.QueryRow(ctx, query).Scan(
		&status.ID,
		&status.SyncedAt,
		&status.Status,
		&status.Error,
		&status.Logs,
		&status.IPRemoved,
		&status.IPAdded,
		&status.IPReplaced,
		&status.CreatedAt,
		&status.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get latest sync: %w", err)
	}

	return &status, nil
}

// GetCurrentSync gets the currently in-progress sync
func (r *WebshareRepository) GetCurrentSync(ctx context.Context) (*models.WebshareSyncStatus, error) {
	query := `
		SELECT id, synced_at, status, error, logs, ip_removed, ip_added, ip_replaced, created_at, updated_at
		FROM webshare_sync_status
		WHERE status = 'IN-PROGRESS'
		ORDER BY synced_at DESC
		LIMIT 1
	`

	var status models.WebshareSyncStatus
	err := r.db.Pool.QueryRow(ctx, query).Scan(
		&status.ID,
		&status.SyncedAt,
		&status.Status,
		&status.Error,
		&status.Logs,
		&status.IPRemoved,
		&status.IPAdded,
		&status.IPReplaced,
		&status.CreatedAt,
		&status.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get current sync: %w", err)
	}

	return &status, nil
}

// GetSyncByID gets a specific sync record by ID
func (r *WebshareRepository) GetSyncByID(ctx context.Context, id int) (*models.WebshareSyncStatus, error) {
	query := `
		SELECT id, synced_at, status, error, logs, ip_removed, ip_added, ip_replaced, created_at, updated_at
		FROM webshare_sync_status
		WHERE id = $1
	`

	var status models.WebshareSyncStatus
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&status.ID,
		&status.SyncedAt,
		&status.Status,
		&status.Error,
		&status.Logs,
		&status.IPRemoved,
		&status.IPAdded,
		&status.IPReplaced,
		&status.CreatedAt,
		&status.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get sync by ID: %w", err)
	}

	return &status, nil
}

// parseJSONArray parses a JSON array string into a slice of strings
func parseJSONArray(jsonStr *string) ([]string, error) {
	if jsonStr == nil || *jsonStr == "" {
		return []string{}, nil
	}

	var arr []string
	if err := json.Unmarshal([]byte(*jsonStr), &arr); err != nil {
		return nil, fmt.Errorf("failed to parse JSON array: %w", err)
	}

	return arr, nil
}

// ToSyncInfo converts a WebshareSyncStatus to WebshareSyncInfo
func (r *WebshareRepository) ToSyncInfo(status *models.WebshareSyncStatus) (*models.WebshareSyncInfo, error) {
	if status == nil {
		return nil, nil
	}

	ipRemoved, _ := parseJSONArray(status.IPRemoved)
	ipAdded, _ := parseJSONArray(status.IPAdded)
	ipReplaced, _ := parseJSONArray(status.IPReplaced)

	return &models.WebshareSyncInfo{
		SyncedAt:   status.SyncedAt,
		Status:     status.Status,
		IPRemoved:  ipRemoved,
		IPAdded:    ipAdded,
		IPReplaced: ipReplaced,
	}, nil
}
