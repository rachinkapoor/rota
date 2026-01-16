package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/alpkeskin/rota/core/internal/models"
	"github.com/alpkeskin/rota/core/internal/repository"
	"github.com/alpkeskin/rota/core/internal/services"
	"github.com/alpkeskin/rota/core/pkg/logger"
)

// WebshareHandler handles Webshare sync endpoints
type WebshareHandler struct {
	syncService  *services.WebshareSyncService
	webshareRepo *repository.WebshareRepository
	hasAPIKey    bool
	syncInterval int
	logger       *logger.Logger
}

// NewWebshareHandler creates a new WebshareHandler
func NewWebshareHandler(
	syncService *services.WebshareSyncService,
	webshareRepo *repository.WebshareRepository,
	hasAPIKey bool,
	syncInterval int,
	log *logger.Logger,
) *WebshareHandler {
	return &WebshareHandler{
		syncService:  syncService,
		webshareRepo: webshareRepo,
		hasAPIKey:    hasAPIKey,
		syncInterval: syncInterval,
		logger:       log,
	}
}

// Sync triggers a manual sync
//	@Summary		Trigger Webshare sync
//	@Description	Manually trigger synchronization with Webshare API
//	@Tags			webshare
//	@Produce		json
//	@Success		200	{object}	models.WebshareSyncResponse	"Sync status"
//	@Failure		400	{object}	models.ErrorResponse
//	@Failure		409	{object}	models.ErrorResponse
//	@Failure		500	{object}	models.ErrorResponse
//	@Router			/webshare/sync [post]
func (h *WebshareHandler) Sync(w http.ResponseWriter, r *http.Request) {
	if !h.hasAPIKey || h.syncService == nil {
		h.errorResponse(w, http.StatusBadRequest, "Webshare API key not configured")
		return
	}

	// Check if sync is already in progress
	if h.syncService.IsSyncing() {
		response := models.WebshareSyncResponse{
			Status:  "already_running",
			Message: "Sync is already in progress",
		}
		h.jsonResponse(w, http.StatusConflict, response)
		return
	}

	// Trigger sync in background
	go func() {
		ctx := context.Background()
		if err := h.syncService.Sync(ctx); err != nil {
			h.logger.Error("sync failed", "error", err)
		}
	}()

	response := models.WebshareSyncResponse{
		Status:  "started",
		Message: "Sync started successfully",
	}
	h.jsonResponse(w, http.StatusOK, response)
}

// GetStatus gets the current sync status
//	@Summary		Get Webshare sync status
//	@Description	Get the current status of Webshare synchronization
//	@Tags			webshare
//	@Produce		json
//	@Success		200	{object}	models.WebshareSyncStatusResponse	"Sync status"
//	@Failure		500	{object}	models.ErrorResponse
//	@Router			/webshare/sync/status [get]
func (h *WebshareHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	response := models.WebshareSyncStatusResponse{
		HasAPIKey: h.hasAPIKey,
	}

	// Get last sync (completed)
	lastSync, err := h.webshareRepo.GetLatestSync(ctx)
	if err != nil {
		h.logger.Error("failed to get latest sync", "error", err)
		h.errorResponse(w, http.StatusInternalServerError, "Failed to get sync status")
		return
	}

	if lastSync != nil {
		lastSyncInfo, err := h.webshareRepo.ToSyncInfo(lastSync)
		if err == nil {
			response.LastSync = lastSyncInfo
		}
	}

	// Get current sync (in-progress)
	currentSync, err := h.webshareRepo.GetCurrentSync(ctx)
	if err != nil {
		h.logger.Error("failed to get current sync", "error", err)
		h.errorResponse(w, http.StatusInternalServerError, "Failed to get sync status")
		return
	}

	if currentSync != nil {
		currentSyncInfo, err := h.webshareRepo.ToSyncInfo(currentSync)
		if err == nil {
			response.CurrentSync = currentSyncInfo
		}
	}

	// Calculate next sync time
	if lastSync != nil && h.syncInterval > 0 {
		nextSyncTime := lastSync.SyncedAt.Add(time.Duration(h.syncInterval) * time.Second)
		response.NextSyncTime = &nextSyncTime
	}

	h.jsonResponse(w, http.StatusOK, response)
}

// jsonResponse sends a JSON response
func (h *WebshareHandler) jsonResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// errorResponse sends an error JSON response
func (h *WebshareHandler) errorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := models.ErrorResponse{
		Error: message,
	}
	h.jsonResponse(w, statusCode, response)
}
