# Rate-Limited Rotation Strategy - Usage Guide

## Overview

The **rate-limited** rotation strategy enforces a configurable maximum number of requests per proxy within a time window. This ensures no proxy exceeds its rate limit, preventing IP bans and maintaining compliance with API rate limits.

## Configuration

### Settings Structure

The rate-limited rotation is configured via the `rotation` settings:

```json
{
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
}
```

### Configuration Parameters

#### `method` (required)
- **Value**: `"rate-limited"` or `"rate_limited"`
- **Description**: Enables the rate-limited rotation strategy

#### `rate_limited` (optional, with defaults)
- **`max_requests_per_minute`** (integer, default: 30)
  - Maximum number of requests allowed per proxy within the time window
  - **Your configurable `n` value**
  - Must be > 0

- **`window_seconds`** (integer, default: 60)
  - Time window in seconds for rate limiting
  - Default is 60 seconds (1 minute)
  - Must be > 0

#### Other Rotation Settings
- **`remove_unhealthy`**: Automatically remove failed proxies from rotation
- **`fallback`**: Enable fallback to other proxies on failure
- **`fallback_max_retries`**: Maximum retries with different proxies
- **`timeout`**: Request timeout in seconds
- **`retries`**: Per-proxy retry attempts

## Usage Examples

### Example 1: 30 Requests per Minute (Your Use Case)

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
      "timeout": 90,
      "retries": 3
    }
  }'
```

**Result**: Each proxy will handle exactly 30 requests per 60-second window, then be excluded until the window resets.

### Example 2: 50 Requests per 2 Minutes

```bash
curl -X PUT http://localhost:8001/api/v1/settings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "rotation": {
      "method": "rate-limited",
      "rate_limited": {
        "max_requests_per_minute": 50,
        "window_seconds": 120
      }
    }
  }'
```

**Result**: Each proxy can handle 50 requests within any 2-minute window.

### Example 3: 10 Requests per 30 Seconds

```bash
curl -X PUT http://localhost:8001/api/v1/settings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "rotation": {
      "method": "rate-limited",
      "rate_limited": {
        "max_requests_per_minute": 10,
        "window_seconds": 30
      }
    }
  }'
```

**Result**: Each proxy can handle 10 requests within any 30-second window.

### Example 4: Using Defaults (30 requests per 60 seconds)

```bash
curl -X PUT http://localhost:8001/api/v1/settings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "rotation": {
      "method": "rate-limited"
    }
  }'
```

**Result**: Uses default values (30 requests per 60 seconds).

## How It Works

### Selection Algorithm

1. **Query Database**: On each proxy selection, the system queries the `proxy_requests` hypertable for requests in the last `window_seconds` per proxy
2. **Filter Proxies**: Excludes proxies that have reached `max_requests_per_minute` in the current window
3. **Round-Robin Selection**: Selects from available proxies using round-robin
4. **Caching**: Results are cached for 2 seconds to reduce database load
5. **Error Handling**: If all proxies are at limit, returns an error

### Database Query

The selector queries:
```sql
SELECT proxy_id, COUNT(*) as request_count
FROM proxy_requests
WHERE proxy_id = ANY($1)
  AND timestamp >= NOW() - INTERVAL '60 seconds'
  AND success = true
GROUP BY proxy_id
HAVING COUNT(*) < 30
```

### Caching Strategy

- **Cache Duration**: 2 seconds (or `window_seconds/5` for very short windows)
- **Cache Invalidation**: On proxy refresh or cache expiry
- **Purpose**: Reduces database query overhead while maintaining accuracy

## Behavior

### Normal Operation

1. Request comes in → Select proxy
2. System checks if proxy has < 30 requests in last 60 seconds
3. If available → Use proxy
4. If not available → Select next available proxy
5. Request is routed through selected proxy
6. Request is recorded in `proxy_requests` table

### Edge Cases

#### All Proxies at Limit

**Scenario**: All 100 proxies have reached 30 requests in the last 60 seconds

**Behavior**: 
- Returns error: `"all proxies have reached rate limit (30 requests/60 seconds)"`
- Client should wait or implement retry logic
- Proxies become available as requests age out of the time window

**Solution**: 
- Increase `max_requests_per_minute` if you have higher limits
- Reduce `window_seconds` for faster recovery
- Add more proxies to the pool

#### Database Query Failure

**Scenario**: Database query fails (connection issue, timeout)

**Behavior**:
- Falls back to using all proxies (logs warning)
- Ensures service continues operating
- May temporarily exceed rate limits

**Solution**:
- Monitor database health
- Ensure proper database connection pooling
- Check logs for query failures

#### Proxy Refresh

**Scenario**: Proxy list is refreshed (new proxies added, unhealthy removed)

**Behavior**:
- Cache is invalidated
- New proxy list is loaded
- Next selection uses fresh data

## Monitoring

### Dashboard

Monitor via the web dashboard at `http://localhost:3000`:
- View proxy request counts
- Check proxy status
- Monitor request distribution

### API Endpoints

#### Get Current Settings
```bash
curl http://localhost:8001/api/v1/settings \
  -H "Authorization: Bearer YOUR_TOKEN"
```

#### Get Proxy List with Stats
```bash
curl http://localhost:8001/api/v1/proxies \
  -H "Authorization: Bearer YOUR_TOKEN"
```

#### Query Request Distribution

You can query the database directly to see request distribution:

```sql
-- Requests per proxy in last 60 seconds
SELECT 
  proxy_id,
  proxy_address,
  COUNT(*) as requests_in_last_minute
FROM proxy_requests
WHERE timestamp >= NOW() - INTERVAL '60 seconds'
  AND success = true
GROUP BY proxy_id, proxy_address
ORDER BY requests_in_last_minute DESC;
```

## Performance Considerations

### Database Load

- **Query Frequency**: Once per proxy selection (cached for 2 seconds)
- **Query Complexity**: O(n) where n = number of proxies
- **Optimization**: 
  - Index on `proxy_requests(proxy_id, timestamp)`
  - Caching reduces query frequency by ~50x

### Recommended Index

Ensure this index exists (should be created by migrations):

```sql
CREATE INDEX IF NOT EXISTS idx_proxy_requests_proxy_time 
ON proxy_requests(proxy_id, timestamp DESC);
```

### Scaling

- **100 proxies**: Excellent performance
- **1000 proxies**: Good performance (consider increasing cache duration)
- **10000+ proxies**: May need optimization (batch queries, longer cache)

## Best Practices

### 1. Set Appropriate Limits

- **Too Low**: Proxies become unavailable too quickly
- **Too High**: Risk of hitting actual rate limits
- **Recommended**: Set to 80-90% of actual rate limit

### 2. Monitor Request Distribution

Regularly check that requests are evenly distributed:
```sql
SELECT 
  proxy_id,
  COUNT(*) as total_requests,
  COUNT(*) FILTER (WHERE timestamp >= NOW() - INTERVAL '1 minute') as requests_last_minute
FROM proxy_requests
WHERE timestamp >= NOW() - INTERVAL '1 hour'
GROUP BY proxy_id
ORDER BY requests_last_minute DESC;
```

### 3. Handle Errors Gracefully

When all proxies are at limit:
- Implement exponential backoff
- Log the event for monitoring
- Consider increasing pool size

### 4. Health Checks

Enable health checks to remove unhealthy proxies:
```json
{
  "rotation": {
    "method": "rate-limited",
    "remove_unhealthy": true
  }
}
```

## Troubleshooting

### Issue: "all proxies have reached rate limit"

**Cause**: All proxies have exceeded the rate limit in the time window

**Solutions**:
1. Wait for requests to age out of the window
2. Increase `max_requests_per_minute`
3. Add more proxies to the pool
4. Reduce `window_seconds` (faster recovery)

### Issue: Uneven Request Distribution

**Cause**: Some proxies getting more requests than others

**Solutions**:
1. Check proxy health (unhealthy proxies are excluded)
2. Verify all proxies are in "active" status
3. Check for database query issues
4. Review cache behavior

### Issue: High Database Load

**Cause**: Too many queries to `proxy_requests` table

**Solutions**:
1. Increase cache duration (modify code)
2. Optimize database indexes
3. Consider batch querying
4. Monitor database connection pool

## Migration from Other Strategies

### From Round-Robin

```bash
# Before
{"method": "round-robin"}

# After
{
  "method": "rate-limited",
  "rate_limited": {
    "max_requests_per_minute": 30,
    "window_seconds": 60
  }
}
```

### From Time-Based

```bash
# Before
{
  "method": "time-based",
  "time_based": {"interval": 60}
}

# After
{
  "method": "rate-limited",
  "rate_limited": {
    "max_requests_per_minute": 30,
    "window_seconds": 60
  }
}
```

## API Reference

### Update Settings

**Endpoint**: `PUT /api/v1/settings`

**Request Body**:
```json
{
  "rotation": {
    "method": "rate-limited",
    "rate_limited": {
      "max_requests_per_minute": 30,
      "window_seconds": 60
    }
  }
}
```

**Response**: `200 OK` with updated settings

### Get Settings

**Endpoint**: `GET /api/v1/settings`

**Response**:
```json
{
  "rotation": {
    "method": "rate-limited",
    "rate_limited": {
      "max_requests_per_minute": 30,
      "window_seconds": 60
    },
    ...
  }
}
```

## Summary

The rate-limited rotation strategy provides:

✅ **Configurable rate limits** - Set `n` (max_requests_per_minute) to any value  
✅ **Time-windowed tracking** - Configurable window_seconds  
✅ **Automatic exclusion** - Proxies at limit are automatically excluded  
✅ **Round-robin selection** - Even distribution among available proxies  
✅ **Caching** - Optimized database queries  
✅ **Production-ready** - Handles edge cases and errors gracefully  

Perfect for your use case: **100 proxies, 30 requests/min per proxy, with keep-alive support**.

