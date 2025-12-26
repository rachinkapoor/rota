# Custom Rate-Limited Rotation Strategy Implementation

## Overview

This document provides a detailed implementation guide for adding a custom rotation strategy that enforces exactly **30 requests per minute per proxy**.

## Architecture

The custom strategy will:
1. Query `proxy_requests` hypertable for requests in the last 60 seconds per proxy
2. Filter out proxies that have reached the limit (>= 30 requests)
3. Select from available proxies using round-robin
4. Handle edge cases (all proxies at limit, no proxies available)

## Implementation

### Step 1: Add RateLimitedSelector to rotation.go

Add this new selector type to `core/internal/proxy/rotation.go`:

```go
// RateLimitedSelector selects proxies that haven't exceeded per-minute rate limit
type RateLimitedSelector struct {
	*BaseSelector
	maxRequestsPerMinute int
	windowSeconds        int
	currentIndex         int
	mu                   sync.RWMutex
}

// NewRateLimitedSelector creates a new rate-limited selector
func NewRateLimitedSelector(
	repo *repository.ProxyRepository,
	settings *models.RotationSettings,
	maxRequestsPerMinute int,
	windowSeconds int,
) *RateLimitedSelector {
	return &RateLimitedSelector{
		BaseSelector: &BaseSelector{
			repo:     repo,
			proxies:  make([]*models.Proxy, 0),
			settings: settings,
		},
		maxRequestsPerMinute: maxRequestsPerMinute,
		windowSeconds:        windowSeconds,
		currentIndex:        0,
	}
}

// Select returns a proxy that hasn't exceeded the rate limit
func (s *RateLimitedSelector) Select(ctx context.Context) (*models.Proxy, error) {
	s.mu.RLock()
	allProxies := s.proxies
	currentIdx := s.currentIndex
	s.mu.RUnlock()

	if len(allProxies) == 0 {
		return nil, fmt.Errorf("no proxies available")
	}

	// Get available proxies (those under the rate limit)
	availableProxies, err := s.getAvailableProxies(ctx, allProxies)
	if err != nil {
		// Fallback: if query fails, use all proxies
		s.logger.Warn("failed to query rate limits, using all proxies", "error", err)
		availableProxies = allProxies
	}

	if len(availableProxies) == 0 {
		// All proxies are at limit - wait a bit or return error
		return nil, fmt.Errorf("all proxies have reached rate limit (%d requests/%d seconds)", 
			s.maxRequestsPerMinute, s.windowSeconds)
	}

	// Round-robin selection from available proxies
	s.mu.Lock()
	selectedProxy := availableProxies[s.currentIndex%len(availableProxies)]
	s.currentIndex = (s.currentIndex + 1) % len(availableProxies)
	s.mu.Unlock()

	return selectedProxy, nil
}

// getAvailableProxies queries the database for proxies under the rate limit
func (s *RateLimitedSelector) getAvailableProxies(ctx context.Context, allProxies []*models.Proxy) ([]*models.Proxy, error) {
	if len(allProxies) == 0 {
		return nil, fmt.Errorf("no proxies to check")
	}

	// Build query to count requests per proxy in last windowSeconds
	query := `
		SELECT 
			proxy_id,
			COUNT(*) as request_count
		FROM proxy_requests
		WHERE 
			proxy_id = ANY($1)
			AND timestamp >= NOW() - INTERVAL '%d seconds'
			AND success = true
		GROUP BY proxy_id
		HAVING COUNT(*) < $2
	`

	// Get proxy IDs
	proxyIDs := make([]int, len(allProxies))
	for i, p := range allProxies {
		proxyIDs[i] = p.ID
	}

	// Execute query
	rows, err := s.repo.GetDB().Pool.Query(
		ctx,
		fmt.Sprintf(query, s.windowSeconds),
		proxyIDs,
		s.maxRequestsPerMinute,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query rate limits: %w", err)
	}
	defer rows.Close()

	// Collect proxy IDs that are under limit
	availableProxyIDs := make(map[int]bool)
	for rows.Next() {
		var proxyID int
		var count int64
		if err := rows.Scan(&proxyID, &count); err != nil {
			continue
		}
		availableProxyIDs[proxyID] = true
	}

	// Also include proxies with zero requests (not in results)
	// Filter proxies that are available
	availableProxies := make([]*models.Proxy, 0)
	for _, proxy := range allProxies {
		if _, exists := availableProxyIDs[proxy.ID]; exists || !s.proxyHasRecentRequests(ctx, proxy.ID) {
			availableProxies = append(availableProxies, proxy)
		}
	}

	return availableProxies, nil
}

// proxyHasRecentRequests checks if proxy has any requests in the time window
func (s *RateLimitedSelector) proxyHasRecentRequests(ctx context.Context, proxyID int) bool {
	query := `
		SELECT COUNT(*) 
		FROM proxy_requests 
		WHERE proxy_id = $1 
		AND timestamp >= NOW() - INTERVAL '%d seconds'
		LIMIT 1
	`
	var count int64
	err := s.repo.GetDB().Pool.QueryRow(
		ctx,
		fmt.Sprintf(query, s.windowSeconds),
		proxyID,
	).Scan(&count)
	
	// If query fails or count > 0, assume it has requests
	return err == nil && count > 0
}

// Refresh reloads the proxy list from database
func (s *RateLimitedSelector) Refresh(ctx context.Context) error {
	proxies, err := s.loadActiveProxiesWithSettings(ctx, s.settings)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.proxies = proxies
	// Reset index if out of bounds
	if s.currentIndex >= len(proxies) {
		s.currentIndex = 0
	}
	s.mu.Unlock()

	return nil
}
```

### Step 2: Update NewProxySelector to support rate-limited method

Modify `NewProxySelector` function in `rotation.go`:

```go
// NewProxySelector creates a proxy selector based on settings
func NewProxySelector(repo *repository.ProxyRepository, settings *models.RotationSettings) (ProxySelector, error) {
	switch settings.Method {
	case "random":
		return NewRandomSelector(repo, settings), nil
	case "roundrobin", "round-robin":
		return NewRoundRobinSelector(repo, settings), nil
	case "least_conn", "least-conn", "least_connections":
		return NewLeastConnectionsSelector(repo, settings), nil
	case "time_based", "time-based":
		interval := settings.TimeBased.Interval
		if interval <= 0 {
			interval = 120 // Default 2 minutes
		}
		return NewTimeBasedSelector(repo, settings, interval), nil
	case "rate_limited", "rate-limited":
		// Default: 30 requests per 60 seconds
		maxRequests := 30
		windowSeconds := 60
		// Could be configurable via settings in the future
		return NewRateLimitedSelector(repo, settings, maxRequests, windowSeconds), nil
	default:
		// Default to random
		return NewRandomSelector(repo, settings), nil
	}
}
```

### Step 3: Add Configuration Options (Optional)

To make it configurable, extend `RotationSettings` in `core/internal/models/settings.go`:

```go
// RotationSettings represents proxy rotation configuration
type RotationSettings struct {
	Method             string            `json:"method"`
	TimeBased          TimeBasedSettings `json:"time_based,omitempty"`
	RateLimited        RateLimitedSettings `json:"rate_limited,omitempty"` // NEW
	// ... existing fields
}

// RateLimitedSettings represents rate-limited rotation settings
type RateLimitedSettings struct {
	MaxRequestsPerMinute int `json:"max_requests_per_minute"` // Default: 30
	WindowSeconds        int `json:"window_seconds"`           // Default: 60
}
```

### Step 4: Update Database Migration (Optional)

If you want to store these settings, update the default rotation settings in `migrations.go`:

```go
// In migration version 4, update the default rotation settings:
('rotation', '{"method": "random", "rate_limited": {"max_requests_per_minute": 30, "window_seconds": 60}, "time_based": {"interval": 120}, "remove_unhealthy": true, "fallback": true, "fallback_max_retries": 10, "follow_redirect": false, "timeout": 90, "retries": 3}'::jsonb),
```

## Usage

### Configuration via API

```bash
curl -X PUT http://localhost:8001/api/v1/settings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "rotation": {
      "method": "rate-limited",
      "rate_limited": {
        "max_requests_per_minute": 30,
        "window_seconds": 60
      },
      "remove_unhealthy": true,
      "fallback": true,
      "fallback_max_retries": 3,
      "timeout": 90,
      "retries": 3
    }
  }'
```

### Configuration via Dashboard

1. Navigate to Settings page
2. Set Rotation Method to "rate-limited"
3. Configure:
   - Max Requests Per Minute: 30
   - Window Seconds: 60

## Performance Considerations

### Database Query Overhead

Each proxy selection requires a database query. To optimize:

1. **Cache results**: Cache available proxies for 1-2 seconds
2. **Batch queries**: Query all proxies at once instead of per-selection
3. **Index optimization**: Ensure `proxy_requests` has proper indexes:
   ```sql
   CREATE INDEX IF NOT EXISTS idx_proxy_requests_proxy_time 
   ON proxy_requests(proxy_id, timestamp DESC);
   ```

### Caching Implementation

Add caching to reduce database load:

```go
type RateLimitedSelector struct {
	// ... existing fields
	cache          []*models.Proxy
	cacheExpiry    time.Time
	cacheDuration  time.Duration
}

func (s *RateLimitedSelector) Select(ctx context.Context) (*models.Proxy, error) {
	// Check cache first
	s.mu.RLock()
	if time.Now().Before(s.cacheExpiry) && len(s.cache) > 0 {
		availableProxies := s.cache
		currentIdx := s.currentIndex
		s.mu.RUnlock()
		
		selectedProxy := availableProxies[currentIdx%len(availableProxies)]
		s.mu.Lock()
		s.currentIndex = (s.currentIndex + 1) % len(availableProxies)
		s.mu.Unlock()
		return selectedProxy, nil
	}
	s.mu.RUnlock()

	// Cache expired or empty, refresh
	availableProxies, err := s.getAvailableProxies(ctx, s.proxies)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache = availableProxies
	s.cacheExpiry = time.Now().Add(s.cacheDuration) // e.g., 2 seconds
	s.mu.Unlock()

	// Select from refreshed cache
	return s.Select(ctx) // Recursive call with fresh cache
}
```

## Testing

### Unit Tests

```go
func TestRateLimitedSelector(t *testing.T) {
	// Setup test database
	// Insert test proxies
	// Insert test requests in proxy_requests
	
	selector := NewRateLimitedSelector(repo, settings, 30, 60)
	
	// Test: Select proxy under limit
	proxy, err := selector.Select(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, proxy)
	
	// Test: After 30 requests, proxy should be excluded
	// ... make 30 requests through same proxy
	availableProxies, _ := selector.getAvailableProxies(ctx, allProxies)
	assert.NotContains(t, availableProxies, proxy)
}
```

### Integration Tests

1. Add 100 proxies to rota
2. Set rotation method to "rate-limited"
3. Send requests at various rates
4. Verify each proxy gets exactly 30 requests per minute
5. Monitor `proxy_requests` table for distribution

## Monitoring

### Metrics to Track

1. **Proxy selection failures**: When all proxies are at limit
2. **Database query latency**: For `getAvailableProxies` queries
3. **Cache hit rate**: If caching is implemented
4. **Request distribution**: Verify 30 requests/min per proxy

### Logging

Add detailed logging:

```go
s.logger.Info("rate-limited proxy selection",
	"available_proxies", len(availableProxies),
	"total_proxies", len(allProxies),
	"selected_proxy_id", selectedProxy.ID,
	"cache_hit", cached,
)
```

## Edge Cases

### All Proxies at Limit

**Solution**: Return error or wait. Could implement a wait mechanism:

```go
if len(availableProxies) == 0 {
	// Wait for oldest request to expire
	oldestRequestTime := s.getOldestRequestTime(ctx)
	waitTime := time.Until(oldestRequestTime.Add(time.Duration(s.windowSeconds) * time.Second))
	if waitTime > 0 && waitTime < 5*time.Second {
		time.Sleep(waitTime)
		return s.Select(ctx) // Retry
	}
	return nil, fmt.Errorf("all proxies at rate limit")
}
```

### Database Query Failure

**Solution**: Fallback to all proxies or least-used proxy:

```go
availableProxies, err := s.getAvailableProxies(ctx, allProxies)
if err != nil {
	s.logger.Warn("rate limit query failed, using least connections", "error", err)
	// Fallback to least connections
	leastConnSelector := NewLeastConnectionsSelector(s.repo, s.settings)
	return leastConnSelector.Select(ctx)
}
```

## Deployment

1. **Build and test** the changes locally
2. **Run database migrations** if you added new settings
3. **Deploy** updated rota core service
4. **Update settings** via API to use "rate-limited" method
5. **Monitor** proxy selection and request distribution
6. **Adjust** `maxRequestsPerMinute` and `windowSeconds` as needed

## Conclusion

This custom rotation strategy provides exact control over per-proxy rate limits, ensuring no proxy exceeds 30 requests per minute. The implementation leverages rota's existing `proxy_requests` hypertable for efficient time-windowed queries.

