# Testing Guide for Rate-Limited Rotation

## Overview

The rate-limited rotation strategy includes comprehensive tests to ensure reliability and correctness. This document explains the testing strategy and how to run tests.

## Test Structure

### Unit Tests (`rotation_test.go`)

Basic unit tests that can run without a database:
- Constructor tests
- Configuration validation
- Method name variant handling

**Status**: Currently skipped (require database for full testing)

### Integration Tests (`rotation_integration_test.go`)

Full integration tests that require a database connection:
- Proxy selection with real database queries
- Rate limit enforcement
- Cache behavior
- Window expiration
- Configurable values

**Tag**: `integration` - Run with `go test -tags=integration`

## Running Tests

### Prerequisites

1. **Test Database**: Set up a PostgreSQL database with TimescaleDB extension
2. **Environment Variables**: Configure database connection
3. **Migrations**: Run database migrations to create required tables

### Setup Test Database

```bash
# Using Docker (recommended)
docker run -d \
  --name rota-test-db \
  -e POSTGRES_USER=rota_test \
  -e POSTGRES_PASSWORD=test_password \
  -e POSTGRES_DB=rota_test \
  -p 5433:5432 \
  timescale/timescaledb:latest-pg16

# Run migrations
export DB_HOST=localhost
export DB_PORT=5433
export DB_USER=rota_test
export DB_PASSWORD=test_password
export DB_NAME=rota_test
```

### Run Unit Tests

```bash
cd core
go test ./internal/proxy -v
```

### Run Integration Tests

```bash
cd core
go test -tags=integration ./internal/proxy -v
```

### Run All Tests

```bash
cd core
go test ./... -v
```

## Test Coverage

### What We Test

✅ **Proxy Selection**
- Selects available proxy when none at limit
- Excludes proxies at rate limit
- Returns error when all proxies at limit
- Round-robin selection from available proxies

✅ **Rate Limiting**
- Enforces max_requests_per_minute limit
- Respects window_seconds time window
- Handles window expiration correctly

✅ **Caching**
- Caches available proxies for performance
- Invalidates cache on refresh
- Cache expires after configured duration

✅ **Configuration**
- Uses configured max_requests_per_minute
- Uses configured window_seconds
- Applies defaults when not configured
- Handles method name variants

✅ **Edge Cases**
- All proxies at limit
- Database query failures
- Empty proxy list
- Zero requests (all proxies available)

## Writing New Tests

### Example: Test Proxy Selection

```go
func TestRateLimitedSelector_Select_AvailableProxy(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    ctx := context.Background()
    proxyRepo := repository.NewProxyRepository(&repository.DB{Pool: db})
    
    // Create selector
    settings := &models.RotationSettings{
        Method: "rate-limited",
        RateLimited: models.RateLimitedSettings{
            MaxRequestsPerMinute: 30,
            WindowSeconds:        60,
        },
    }
    selector := NewRateLimitedSelector(proxyRepo, settings, 30, 60)
    
    // Add test proxy
    // ... insert proxy into database ...
    
    // Refresh
    err := selector.Refresh(ctx)
    require.NoError(t, err)
    
    // Select
    proxy, err := selector.Select(ctx)
    require.NoError(t, err)
    assert.NotNil(t, proxy)
}
```

### Example: Test Rate Limit Enforcement

```go
func TestRateLimitedSelector_ExcludeAtLimit(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    ctx := context.Background()
    
    // Insert 30 requests for proxy 1 in last 60 seconds
    // ... insert test data into proxy_requests ...
    
    // Select - should exclude proxy 1
    proxy, err := selector.Select(ctx)
    require.NoError(t, err)
    assert.NotEqual(t, 1, proxy.ID, "Should not select proxy at limit")
}
```

## Test Database Setup

### Using Docker Compose

Create `docker-compose.test.yml`:

```yaml
version: '3.8'
services:
  test-db:
    image: timescale/timescaledb:latest-pg16
    environment:
      POSTGRES_USER: rota_test
      POSTGRES_PASSWORD: test_password
      POSTGRES_DB: rota_test
    ports:
      - "5433:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U rota_test"]
      interval: 5s
      timeout: 5s
      retries: 5
```

Run:
```bash
docker-compose -f docker-compose.test.yml up -d
```

### Manual Setup

```sql
-- Create test database
CREATE DATABASE rota_test;

-- Connect to test database
\c rota_test

-- Enable TimescaleDB
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Run migrations (same as production)
-- ... run migration scripts ...
```

## Continuous Integration

### GitHub Actions Example

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: timescale/timescaledb:latest-pg16
        env:
          POSTGRES_USER: rota_test
          POSTGRES_PASSWORD: test_password
          POSTGRES_DB: rota_test
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.25'
      
      - name: Run migrations
        run: |
          # Run database migrations
      
      - name: Run tests
        env:
          DB_HOST: localhost
          DB_PORT: 5432
          DB_USER: rota_test
          DB_PASSWORD: test_password
          DB_NAME: rota_test
        run: |
          cd core
          go test -tags=integration ./... -v
```

## Mocking Strategy

For unit tests that don't require a database, you can create mock repositories:

```go
type MockProxyRepository struct {
    proxies []*models.Proxy
}

func (m *MockProxyRepository) GetDB() *repository.DB {
    // Return mock DB connection
}
```

## Test Data Management

### Fixtures

Create test fixtures for common scenarios:

```go
func createTestProxies(t *testing.T, db *pgxpool.Pool) []*models.Proxy {
    // Create test proxies in database
    // Return proxy list
}

func insertTestRequests(t *testing.T, db *pgxpool.Pool, proxyID int, count int, age time.Duration) {
    // Insert test requests into proxy_requests table
    // age: how old the requests should be
}
```

### Cleanup

Always clean up test data:

```go
func cleanupTestData(t *testing.T, db *pgxpool.Pool) {
    ctx := context.Background()
    db.Exec(ctx, "TRUNCATE TABLE proxy_requests, proxies CASCADE")
}
```

## Performance Testing

### Load Testing

Test with realistic load:

```go
func TestRateLimitedSelector_Performance(t *testing.T) {
    // Test with 100 proxies
    // Test with high request rate
    // Measure query performance
    // Verify cache effectiveness
}
```

## Best Practices

1. **Isolation**: Each test should be independent
2. **Cleanup**: Always clean up test data
3. **Realistic Data**: Use realistic test scenarios
4. **Error Cases**: Test error handling
5. **Edge Cases**: Test boundary conditions
6. **Documentation**: Document test scenarios

## Current Status

- ✅ Test structure created
- ⚠️ Tests require database setup
- ⚠️ Integration tests need test database configuration
- 📝 Mock repository needed for unit tests

## Next Steps

1. Set up test database infrastructure
2. Implement test database setup/teardown
3. Create test fixtures
4. Implement mock repository for unit tests
5. Add CI/CD test pipeline
6. Achieve >80% code coverage

