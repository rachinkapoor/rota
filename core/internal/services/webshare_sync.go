package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alpkeskin/rota/core/internal/models"
	"github.com/alpkeskin/rota/core/internal/proxy"
	"github.com/alpkeskin/rota/core/internal/repository"
	"github.com/alpkeskin/rota/core/pkg/logger"
)

// WebshareSyncService handles synchronization between Webshare and ROTA
type WebshareSyncService struct {
	client        *WebshareClient
	webshareRepo  *repository.WebshareRepository
	proxyRepo     *repository.ProxyRepository
	healthChecker *proxy.HealthChecker
	logger        *logger.Logger
	mode          string
	syncInterval  int
	mu            sync.Mutex
	isSyncing     bool
}

// NewWebshareSyncService creates a new WebshareSyncService
func NewWebshareSyncService(
	apiKey string,
	webshareRepo *repository.WebshareRepository,
	proxyRepo *repository.ProxyRepository,
	healthChecker *proxy.HealthChecker,
	mode string,
	syncInterval int,
	log *logger.Logger,
) *WebshareSyncService {
	return &WebshareSyncService{
		client:        NewWebshareClient(apiKey, log),
		webshareRepo: webshareRepo,
		proxyRepo:    proxyRepo,
		healthChecker: healthChecker,
		logger:       log,
		mode:         mode,
		syncInterval: syncInterval,
	}
}

// Sync performs the complete sync workflow
func (s *WebshareSyncService) Sync(ctx context.Context) error {
	// Check if already syncing
	s.mu.Lock()
	if s.isSyncing {
		s.mu.Unlock()
		return fmt.Errorf("sync already in progress")
	}
	s.isSyncing = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isSyncing = false
		s.mu.Unlock()
	}()

	// Step 0: Create sync status record
	syncedAt := time.Now()
	syncStatus, err := s.webshareRepo.CreateSyncStatus(ctx, syncedAt)
	if err != nil {
		return fmt.Errorf("failed to create sync status: %w", err)
	}

	s.addLog(ctx, syncStatus.ID, "info", "Sync started")

	// Step 1: Fetch Webshare Proxies
	s.addLog(ctx, syncStatus.ID, "info", "Fetching proxies from Webshare")
	webshareProxies, err := s.client.ListProxies(ctx, s.mode)
	if err != nil {
		s.updateSyncStatus(ctx, syncStatus.ID, "FAILED", nil, nil, nil, nil, nil)
		s.addError(ctx, syncStatus.ID, "fetch_failed", "", fmt.Sprintf("Failed to fetch Webshare proxies: %v", err))
		return fmt.Errorf("failed to fetch Webshare proxies: %w", err)
	}

	s.addLog(ctx, syncStatus.ID, "info", fmt.Sprintf("Fetched %d proxies from Webshare", len(webshareProxies)))

	// Build maps for comparison
	webshareIPMap := make(map[string]models.WebshareProxy)
	webshareUnhealthyIPs := []string{}
	for _, p := range webshareProxies {
		ip := fmt.Sprintf("%s:%d", p.ProxyAddress, p.Port)
		webshareIPMap[ip] = p
		if !p.Valid {
			webshareUnhealthyIPs = append(webshareUnhealthyIPs, ip)
		}
	}

	// Get all ROTA proxies
	rotaProxies, _, err := s.proxyRepo.List(ctx, 1, 10000, "", "", "", "created_at", "asc")
	if err != nil {
		s.updateSyncStatus(ctx, syncStatus.ID, "FAILED", nil, nil, nil, nil, nil)
		s.addError(ctx, syncStatus.ID, "fetch_rota_failed", "", fmt.Sprintf("Failed to fetch ROTA proxies: %v", err))
		return fmt.Errorf("failed to fetch ROTA proxies: %w", err)
	}

	// Step 2: Remove Missing IPs from ROTA
	s.addLog(ctx, syncStatus.ID, "info", "Removing IPs not in Webshare")
	ipRemoved := []string{}
	for _, p := range rotaProxies {
		if _, exists := webshareIPMap[p.Address]; !exists {
			if err := s.proxyRepo.Delete(ctx, p.ID); err != nil {
				s.addError(ctx, syncStatus.ID, "remove_failed", p.Address, fmt.Sprintf("Failed to remove proxy: %v", err))
			} else {
				ipRemoved = append(ipRemoved, p.Address)
				s.addLog(ctx, syncStatus.ID, "info", fmt.Sprintf("Removed proxy: %s", p.Address))
			}
		}
	}

	// Step 3: Add New IPs to ROTA
	s.addLog(ctx, syncStatus.ID, "info", "Adding new IPs from Webshare")
	ipAdded := []string{}
	newProxies := []*models.Proxy{}
	
	// Build ROTA proxy address map for efficient lookup
	rotaAddressMap := make(map[string]bool)
	for _, rp := range rotaProxies {
		rotaAddressMap[rp.Address] = true
	}
	
	for _, wsProxy := range webshareProxies {
		ip := fmt.Sprintf("%s:%d", wsProxy.ProxyAddress, wsProxy.Port)
		
		// Check if already exists in ROTA
		if !rotaAddressMap[ip] {
			// Determine protocol from Webshare (default to http)
			protocol := "http"
			// Webshare doesn't provide protocol in API, so we'll default to http
			
			req := models.CreateProxyRequest{
				Address:  ip,
				Protocol: protocol,
				Username: &wsProxy.Username,
				Password: &wsProxy.Password,
			}

			proxy, err := s.proxyRepo.Create(ctx, req)
			if err != nil {
				// If it's a duplicate error, skip it
				if strings.Contains(err.Error(), "already exists") {
					continue
				}
				s.addError(ctx, syncStatus.ID, "add_failed", ip, fmt.Sprintf("Failed to add proxy: %v", err))
			} else {
				ipAdded = append(ipAdded, ip)
				newProxies = append(newProxies, proxy)
				s.addLog(ctx, syncStatus.ID, "info", fmt.Sprintf("Added proxy: %s", ip))
			}
		}
	}

	// Step 4: Health Check Newly Added IPs
	if len(newProxies) > 0 {
		s.addLog(ctx, syncStatus.ID, "info", fmt.Sprintf("Health checking %d newly added IPs", len(newProxies)))
		for _, p := range newProxies {
			result, err := s.healthChecker.CheckProxy(ctx, p)
			if err != nil {
				s.addError(ctx, syncStatus.ID, "health_check_failed", p.Address, fmt.Sprintf("Health check error: %v", err))
			} else if result.Status == "failed" {
				s.addLog(ctx, syncStatus.ID, "warning", fmt.Sprintf("New proxy %s failed health check", p.Address))
			} else {
				s.addLog(ctx, syncStatus.ID, "info", fmt.Sprintf("New proxy %s passed health check", p.Address))
			}
		}
	}

	// Step 5: Handle Webshare Unhealthy IPs
	ipReplaced := []string{}
	if len(webshareUnhealthyIPs) > 0 {
		s.addLog(ctx, syncStatus.ID, "info", fmt.Sprintf("Requesting replacement for %d unhealthy Webshare IPs", len(webshareUnhealthyIPs)))
		ipReplaced = append(ipReplaced, webshareUnhealthyIPs...)
		
		// Build ROTA proxy map for efficient lookup
		rotaProxyMap := make(map[string]*models.ProxyWithStats)
		for i := range rotaProxies {
			rotaProxyMap[rotaProxies[i].Address] = &rotaProxies[i]
		}
		
		// Request replacement (async, fire and forget)
		syncID := syncStatus.ID
		go func() {
			_, err := s.client.CreateReplacement(context.Background(), webshareUnhealthyIPs)
			if err != nil {
				s.logger.Error("failed to request replacement for Webshare unhealthy IPs", "error", err, "ip_count", len(webshareUnhealthyIPs))
				// Add error to sync status for each IP in the batch
				for _, ip := range webshareUnhealthyIPs {
					s.addError(context.Background(), syncID, "replacement_failed", ip, fmt.Sprintf("Failed to request replacement from Webshare: %v", err))
				}
			} else {
				s.logger.Info("replacement requested successfully for Webshare unhealthy IPs", "ip_count", len(webshareUnhealthyIPs))
			}
		}()

		// Remove unhealthy IPs from ROTA
		for _, ip := range webshareUnhealthyIPs {
			if proxy, exists := rotaProxyMap[ip]; exists {
				if err := s.proxyRepo.Delete(ctx, proxy.ID); err != nil {
					s.addError(ctx, syncStatus.ID, "remove_unhealthy_failed", ip, fmt.Sprintf("Failed to remove unhealthy proxy: %v", err))
				} else {
					s.addLog(ctx, syncStatus.ID, "info", fmt.Sprintf("Removed unhealthy proxy: %s", ip))
				}
			} else {
				s.addLog(ctx, syncStatus.ID, "warning", fmt.Sprintf("Unhealthy proxy %s not found in ROTA, skipping removal", ip))
			}
		}
	}

	// Step 6: Post-Sync: Check ROTA Unhealthy IPs
	s.addLog(ctx, syncStatus.ID, "info", "Checking ROTA unhealthy IPs")
	rotaFailedProxies, _, err := s.proxyRepo.List(ctx, 1, 10000, "failed", "", "", "created_at", "asc")
	if err == nil && len(rotaFailedProxies) > 0 {
		rotaFailedIPs := []string{}
		for _, p := range rotaFailedProxies {
			rotaFailedIPs = append(rotaFailedIPs, p.Address)
		}

		if len(rotaFailedIPs) > 0 {
			s.addLog(ctx, syncStatus.ID, "info", fmt.Sprintf("Requesting replacement for %d ROTA unhealthy IPs", len(rotaFailedIPs)))
			ipReplaced = append(ipReplaced, rotaFailedIPs...)

			// Request replacement (async, fire and forget)
			syncID := syncStatus.ID
			go func() {
				_, err := s.client.CreateReplacement(context.Background(), rotaFailedIPs)
				if err != nil {
					s.logger.Error("failed to request replacement for ROTA unhealthy IPs", "error", err, "ip_count", len(rotaFailedIPs))
					// Add error to sync status for each IP in the batch
					for _, ip := range rotaFailedIPs {
						s.addError(context.Background(), syncID, "replacement_failed", ip, fmt.Sprintf("Failed to request replacement from Webshare: %v", err))
					}
				} else {
					s.logger.Info("replacement requested successfully for ROTA unhealthy IPs", "ip_count", len(rotaFailedIPs))
				}
			}()
		}
	}

	// Update sync status with final results
	ipRemovedJSON := s.arrayToJSON(ipRemoved)
	ipAddedJSON := s.arrayToJSON(ipAdded)
	ipReplacedJSON := s.arrayToJSON(ipReplaced)

	s.updateSyncStatus(ctx, syncStatus.ID, "SUCCESS", nil, nil, &ipRemovedJSON, &ipAddedJSON, &ipReplacedJSON)
	s.addLog(ctx, syncStatus.ID, "info", "Sync completed successfully")

	return nil
}

// IsSyncing returns whether a sync is currently in progress
func (s *WebshareSyncService) IsSyncing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isSyncing
}

// addLog adds a log entry to the sync status
func (s *WebshareSyncService) addLog(ctx context.Context, syncID int, level, message string) {
	status, err := s.webshareRepo.GetSyncByID(ctx, syncID)
	if err != nil || status == nil {
		return
	}

	logs := []models.WebshareLog{}
	if status.Logs != nil {
		json.Unmarshal([]byte(*status.Logs), &logs)
	}

	logs = append(logs, models.WebshareLog{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	})

	logsJSON, _ := json.Marshal(logs)
	logsStr := string(logsJSON)
	s.webshareRepo.UpdateSyncStatus(ctx, syncID, status.Status, status.Error, &logsStr, status.IPRemoved, status.IPAdded, status.IPReplaced)
}

// addError adds an error entry to the sync status
func (s *WebshareSyncService) addError(ctx context.Context, syncID int, errorType, ip, message string) {
	status, err := s.webshareRepo.GetSyncByID(ctx, syncID)
	if err != nil || status == nil {
		return
	}

	errors := []models.WebshareError{}
	if status.Error != nil {
		json.Unmarshal([]byte(*status.Error), &errors)
	}

	errors = append(errors, models.WebshareError{
		Type:    errorType,
		IP:      ip,
		Message: message,
	})

	errorsJSON, _ := json.Marshal(errors)
	errorsStr := string(errorsJSON)
	s.webshareRepo.UpdateSyncStatus(ctx, syncID, status.Status, &errorsStr, status.Logs, status.IPRemoved, status.IPAdded, status.IPReplaced)
}

// updateSyncStatus updates the sync status record
func (s *WebshareSyncService) updateSyncStatus(ctx context.Context, syncID int, status string, errorJSON, logsJSON, ipRemoved, ipAdded, ipReplaced *string) {
	s.webshareRepo.UpdateSyncStatus(ctx, syncID, status, errorJSON, logsJSON, ipRemoved, ipAdded, ipReplaced)
}

// arrayToJSON converts a string array to JSON string
func (s *WebshareSyncService) arrayToJSON(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	jsonBytes, _ := json.Marshal(arr)
	return string(jsonBytes)
}
