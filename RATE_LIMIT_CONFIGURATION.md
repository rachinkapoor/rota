# Rota Rate Limit Configuration Guide

## Your Requirements
- **100 proxy addresses** to rotate
- **30 requests/min per IP** (n = 30)
- **Keep-alive header** support (already enabled)
- **Goal**: Serve exactly n requests from each proxy, then rotate

## Problem Analysis

Rota's current architecture:
- Tracks **lifetime request counts** per proxy (not time-windowed)
- Rotation strategies don't query `proxy_requests` hypertable for per-minute limits
- No built-in per-proxy rate limiting based on time windows

## Recommended Configuration

### Option 1: Time-Based Rotation (BEST APPROXIMATION)

This is the closest you can get with existing rota features without code changes.

**Configuration via API (`PUT /api/v1/settings`):**

```json
{
  "rotation": {
    "method": "time-based",
    "time_based": {
      "interval": 60
    },
    "remove_unhealthy": true,
    "fallback": true,
    "fallback_max_retries": 3,
    "follow_redirect": false,
    "timeout": 90,
    "retries": 3,
    "allowed_protocols": [],
    "max_response_time": 0,
    "min_success_rate": 0
  },
  "rate_limit": {
    "enabled": false
  },
  "authentication": {
    "enabled": false
  }
}
```

**How it works:**
- Rotates proxies every 60 seconds (1 minute)
- Each proxy gets selected for 1 minute window
- If you send requests at ~0.5 requests/second (30/min), each proxy will handle ~30 requests per rotation cycle

**Calculation:**
- 100 proxies × 30 requests/min = 3,000 requests/min total capacity
- Time-based rotation: proxy changes every 60 seconds
- With 100 proxies, each proxy is active for 60 seconds every 100 minutes
- **Issue**: This doesn't guarantee exactly 30 requests per proxy per minute

### Option 2: Round-Robin with Request Throttling (CLOSER TO YOUR NEED)

**Configuration:**

```json
{
  "rotation": {
    "method": "round-robin",
    "remove_unhealthy": true,
    "fallback": true,
    "fallback_max_retries": 3,
    "follow_redirect": false,
    "timeout": 90,
    "retries": 3
  },
  "rate_limit": {
    "enabled": true,
    "interval": 60,
    "max_requests": 3000
  }
}
```

**How it works:**
- Round-robin cycles through all 100 proxies sequentially
- Each request goes to next proxy in sequence
- With 100 proxies, after 100 requests, you're back to proxy #1
- **Issue**: Doesn't enforce 30 requests/min per proxy - just distributes evenly

### Option 3: Least Connections Strategy (BALANCED LOAD)

```json
{
  "rotation": {
    "method": "least_connections",
    "remove_unhealthy": true,
    "fallback": true,
    "fallback_max_retries": 3,
    "follow_redirect": false,
    "timeout": 90,
    "retries": 3
  }
}
```

**How it works:**
- Always selects proxy with lowest lifetime request count
- Naturally balances load across proxies
- **Issue**: Doesn't respect time windows - proxies with low lifetime counts get all traffic

## The Core Problem

**None of the existing strategies can guarantee exactly 30 requests per minute per proxy** because:
1. Rota tracks lifetime requests, not time-windowed requests
2. Rotation logic doesn't query `proxy_requests` hypertable for recent request counts
3. No per-proxy rate limiting mechanism exists

## Solution: Custom Rotation Strategy (REQUIRES CODE CHANGE)

To achieve your exact requirement, you need a custom rotation strategy that:

1. **Queries `proxy_requests` hypertable** for requests in last 60 seconds per proxy
2. **Excludes proxies** that have >= 30 requests in current minute
3. **Selects from available proxies** (those with < 30 requests/min)
4. **Falls back** if all proxies are at limit (wait or use least-used)

### Implementation Approach

You would need to modify `core/internal/proxy/rotation.go` to add:

```go
// RateLimitedSelector selects proxies that haven't exceeded per-minute rate limit
type RateLimitedSelector struct {
    *BaseSelector
    maxRequestsPerMinute int
    windowSeconds        int
}

func (s *RateLimitedSelector) Select(ctx context.Context) (*models.Proxy, error) {
    // Query proxy_requests for last 60 seconds
    // Filter proxies with < 30 requests in last minute
    // Select from available proxies (round-robin or random)
}
```

## Recommended Workaround (NO CODE CHANGES)

Since you need this working immediately, here's the best workaround:

### Configuration: Round-Robin + External Rate Limiting

1. **Set rota to round-robin:**
```json
{
  "rotation": {
    "method": "round-robin",
    "timeout": 90,
    "retries": 3
  }
}
```

2. **Implement client-side rate limiting:**
   - Track requests per proxy in your application
   - After 30 requests to a proxy, skip it for 1 minute
   - Use a proxy rotation list in your client code

3. **Or use a load balancer in front:**
   - Place a rate-limiting proxy (e.g., nginx with rate limiting) in front of rota
   - Configure it to limit requests per upstream (rota proxy) to 30/min

## Keep-Alive Configuration

Keep-alive is **already enabled** in rota's transport configuration:

```go
// From transport.go
MaxIdleConns:        100,
MaxIdleConnsPerHost: 10,
IdleConnTimeout:     90 * time.Second,
```

This means:
- ✅ HTTP connections are reused (keep-alive)
- ✅ Reduces connection overhead
- ✅ Faster response times

## Pros and Cons

### Time-Based Rotation

**Pros:**
- Simple configuration
- Predictable rotation pattern
- Works with existing rota features

**Cons:**
- ❌ Doesn't guarantee exactly 30 requests per proxy
- ❌ All proxies rotate at same time (not per-proxy)
- ❌ If request rate varies, some proxies may get more/less than 30

### Round-Robin

**Pros:**
- Even distribution across all proxies
- Simple and predictable
- Works with existing features

**Cons:**
- ❌ No per-proxy rate limiting
- ❌ Doesn't respect 30 requests/min limit
- ❌ Requires external rate limiting layer

### Least Connections

**Pros:**
- Natural load balancing
- Adapts to proxy performance
- Works with existing features

**Cons:**
- ❌ Doesn't respect time windows
- ❌ Can starve some proxies if others are slow
- ❌ No guarantee of 30 requests/min per proxy

### Custom Rate-Limited Strategy (Code Change)

**Pros:**
- ✅ Exactly matches your requirement
- ✅ Enforces 30 requests/min per proxy
- ✅ Uses existing `proxy_requests` hypertable
- ✅ No external dependencies

**Cons:**
- ❌ Requires code modification
- ❌ Needs testing and deployment
- ❌ Adds database query overhead (querying `proxy_requests` on each selection)

## My Recommendation

**For immediate use (no code changes):**
1. Use **round-robin** rotation
2. Implement **client-side tracking** of requests per proxy
3. Skip proxies that have reached 30 requests in the last minute
4. This gives you control without modifying rota

**For production (with code changes):**
1. Implement **custom `RateLimitedSelector`** in rota
2. Query `proxy_requests` hypertable for last 60 seconds per proxy
3. Filter out proxies with >= 30 requests
4. Select from available proxies using round-robin
5. This gives you exact control and is the most robust solution

## Configuration Example (Time-Based - Best Approximation)

```bash
curl -X PUT http://localhost:8001/api/v1/settings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "rotation": {
      "method": "time-based",
      "time_based": {
        "interval": 60
      },
      "remove_unhealthy": true,
      "fallback": true,
      "fallback_max_retries": 3,
      "follow_redirect": false,
      "timeout": 90,
      "retries": 3
    }
  }'
```

## Monitoring

Monitor proxy usage via:
- Dashboard: `http://localhost:3000`
- API: `GET /api/v1/proxies` - check `requests` field
- Logs: Check `proxy_requests` hypertable for time-windowed analysis

## Next Steps

1. **Test with time-based rotation** first to see if it meets your needs
2. **Monitor request distribution** per proxy
3. **If not sufficient**, implement custom rotation strategy
4. **Consider** adding per-proxy rate limiting as a feature request to rota

