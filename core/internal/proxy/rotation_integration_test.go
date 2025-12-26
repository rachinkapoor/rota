// +build integration

package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/alpkeskin/rota/core/internal/models"
	"github.com/alpkeskin/rota/core/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests for rate-limited rotation
// These tests require a real database connection
// Run with: go test -tags=integration ./internal/proxy

// setupTestDB creates a test database connection
// In a real scenario, you'd use a test database or docker container
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	// This would connect to a test database
	// For now, we'll skip if no test DB is available
	t.Skip("Test database not configured - set up test database connection")
	return nil
}

// TestRateLimitedSelector_Integration tests the full integration
func TestRateLimitedSelector_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	proxyRepo := repository.NewProxyRepository(&repository.DB{Pool: db})

	// Create test settings
	settings := &models.RotationSettings{
		Method: "rate-limited",
		RateLimited: models.RateLimitedSettings{
			MaxRequestsPerMinute: 30,
			WindowSeconds:        60,
		},
		RemoveUnhealthy: true,
		Fallback:        true,
		Timeout:         90,
		Retries:         3,
	}

	// Create selector
	selector := NewRateLimitedSelector(proxyRepo, settings, 30, 60)

	// Test 1: Select proxy when none have requests
	t.Run("Select_NoRequests", func(t *testing.T) {
		// Add test proxies
		// ... add proxies to database ...

		// Refresh selector
		err := selector.Refresh(ctx)
		if err != nil {
			t.Fatalf("Failed to refresh: %v", err)
		}

		// Select proxy
		proxy, err := selector.Select(ctx)
		if err != nil {
			t.Fatalf("Failed to select proxy: %v", err)
		}
		if proxy == nil {
			t.Fatal("Selected proxy is nil")
		}
	})

	// Test 2: Exclude proxy at limit
	t.Run("Exclude_AtLimit", func(t *testing.T) {
		// Insert 30 requests for proxy 1 in last 60 seconds
		// ... insert test data ...

		// Select proxy - should not return proxy 1
		proxy, err := selector.Select(ctx)
		if err != nil {
			t.Fatalf("Failed to select proxy: %v", err)
		}
		if proxy.ID == 1 {
			t.Error("Selected proxy that should be excluded (at limit)")
		}
	})

	// Test 3: All proxies at limit
	t.Run("AllProxiesAtLimit", func(t *testing.T) {
		// Insert 30 requests for all proxies
		// ... insert test data ...

		// Select proxy - should return error
		_, err := selector.Select(ctx)
		if err == nil {
			t.Error("Expected error when all proxies are at limit")
		}
	})

	// Test 4: Cache behavior
	t.Run("CacheBehavior", func(t *testing.T) {
		// Select proxy multiple times quickly
		// Should use cache for subsequent calls
		// ... test cache ...
	})

	// Test 5: Window expiration
	t.Run("WindowExpiration", func(t *testing.T) {
		// Insert requests older than window
		// Proxy should become available again
		// ... test window expiration ...
	})
}

// TestRateLimitedSelector_ConfigurableValues tests different configurations
func TestRateLimitedSelector_ConfigurableValues(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testCases := []struct {
		name                string
		maxRequestsPerMinute int
		windowSeconds        int
		expectedBehavior     string
	}{
		{"Default_30_60", 30, 60, "30 requests per 60 seconds"},
		{"HighLimit_100_60", 100, 60, "100 requests per 60 seconds"},
		{"ShortWindow_30_30", 30, 30, "30 requests per 30 seconds"},
		{"Custom_50_120", 50, 120, "50 requests per 120 seconds"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()

			ctx := context.Background()
			proxyRepo := repository.NewProxyRepository(&repository.DB{Pool: db})

			settings := &models.RotationSettings{
				Method: "rate-limited",
				RateLimited: models.RateLimitedSettings{
					MaxRequestsPerMinute: tc.maxRequestsPerMinute,
					WindowSeconds:        tc.windowSeconds,
				},
			}

			selector := NewRateLimitedSelector(proxyRepo, settings, tc.maxRequestsPerMinute, tc.windowSeconds)

			// Test that selector uses correct values
			// ... test implementation ...
		})
	}
}

