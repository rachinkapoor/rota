package models

import "time"

// WebshareSyncStatus represents a sync status record in the database
type WebshareSyncStatus struct {
	ID         int        `json:"id"`
	SyncedAt   time.Time  `json:"synced_at"`
	Status     string     `json:"status"` // IN-PROGRESS, FAILED, SUCCESS
	Error      *string    `json:"error,omitempty"` // JSON array string
	Logs       *string    `json:"logs,omitempty"`   // JSON array string
	IPRemoved  *string    `json:"ip_removed,omitempty"`  // JSON array string
	IPAdded    *string    `json:"ip_added,omitempty"`    // JSON array string
	IPReplaced *string    `json:"ip_replaced,omitempty"` // JSON array string
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// WebshareSyncStatusResponse represents the API response for sync status
type WebshareSyncStatusResponse struct {
	LastSync    *WebshareSyncInfo `json:"last_sync,omitempty"`
	CurrentSync *WebshareSyncInfo `json:"current_sync,omitempty"`
	NextSyncTime *time.Time       `json:"next_sync_time,omitempty"`
	HasAPIKey   bool              `json:"has_api_key"`
}

// WebshareSyncInfo represents sync information in the response
type WebshareSyncInfo struct {
	SyncedAt   time.Time `json:"synced_at"`
	Status     string    `json:"status"`
	IPRemoved  []string  `json:"ip_removed,omitempty"`
	IPAdded    []string  `json:"ip_added,omitempty"`
	IPReplaced []string  `json:"ip_replaced,omitempty"`
}

// WebshareSyncRequest represents a request to trigger sync
type WebshareSyncRequest struct {
	// Empty for now, but can be extended in the future
}

// WebshareSyncResponse represents the response from triggering sync
type WebshareSyncResponse struct {
	Status  string `json:"status"` // "started" or "already_running"
	Message string `json:"message"`
}

// WebshareError represents an error entry in the error JSON array
type WebshareError struct {
	Type    string `json:"type"`
	IP      string `json:"ip,omitempty"`
	Message string `json:"message"`
}

// WebshareLog represents a log entry in the logs JSON array
type WebshareLog struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // info, warning, error
	Message   string    `json:"message"`
}

// WebshareProxy represents a proxy from Webshare API
type WebshareProxy struct {
	ProxyAddress string `json:"proxy_address"`
	Port         int    `json:"port"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Valid        bool   `json:"valid"`
}

// WebshareProxyListResponse represents the response from Webshare proxy list API
type WebshareProxyListResponse struct {
	Results []WebshareProxy `json:"results"`
	Next    *string         `json:"next"`
	Count   int             `json:"count"`
}

// WebshareReplacementRequest represents a request to replace proxies
type WebshareReplacementRequest struct {
	ToReplace struct {
		ProxyAddressIn []string `json:"proxy_address__in"`
	} `json:"to_replace"`
	ReplaceWith []map[string]interface{} `json:"replace_with"`
	DryRun      bool                      `json:"dry_run"`
}

// WebshareReplacementResponse represents the response from replacement API
type WebshareReplacementResponse struct {
	ID    int    `json:"id"`
	State string `json:"state"`
}
