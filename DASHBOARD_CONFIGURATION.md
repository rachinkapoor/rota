# Dashboard Configuration for Rate-Limited Rotation

## ✅ Yes, It's Fully Configurable via Dashboard!

The rate-limited rotation strategy is **fully integrated** into the Rota dashboard UI. You can configure all settings without using the API directly.

## Dashboard UI Location

Navigate to: **Settings → Proxy Rotation**

## Configuration Steps

### 1. Select Rate-Limited Method

In the **Rotation Method** dropdown, select:
- **"Rate Limited"** (or it will accept "rate-limited" / "rate_limited")

### 2. Configure Rate Limit Settings

When "Rate Limited" is selected, two new input fields appear:

#### **Max Requests Per Minute**
- **Field**: `max_requests_per_minute`
- **Default**: 30
- **Description**: Maximum number of requests allowed per proxy within the time window
- **Your configurable `n` value** ✅
- **Min Value**: 1

#### **Time Window (seconds)**
- **Field**: `window_seconds`
- **Default**: 60
- **Description**: Time window in seconds for rate limiting
- **Default**: 60 seconds (1 minute)
- **Min Value**: 1

### 3. Configure Other Rotation Settings

You can also configure:
- **Timeout**: Request timeout in seconds (default: 90)
- **Retries**: Per-proxy retry attempts (default: 3)
- **Fallback Max Retries**: Maximum retries with different proxies
- **Remove Unhealthy**: Automatically remove failed proxies
- **Follow Redirect**: Follow HTTP redirections
- **Max Response Time**: Filter slow proxies
- **Min Success Rate**: Filter unreliable proxies
- **Allowed Protocols**: Restrict to specific protocols

### 4. Save Configuration

Click **"Save Configuration"** button to apply changes.

## UI Screenshot Description

When you select "Rate Limited" from the rotation method dropdown, you'll see:

```
┌─────────────────────────────────────────┐
│ Proxy Rotation                          │
├─────────────────────────────────────────┤
│                                         │
│ Rotation Method: [Rate Limited ▼]      │
│                                         │
│ Max Requests Per Minute: [30]          │
│ Maximum number of requests allowed     │
│ per proxy within the time window        │
│                                         │
│ Time Window (seconds): [60]            │
│ Time window in seconds for rate         │
│ limiting (default: 60 = 1 minute)     │
│                                         │
│ Timeout (seconds): [90]                │
│ Retries: [3]                           │
│ ... (other settings)                   │
│                                         │
└─────────────────────────────────────────┘
```

## Example Configuration for Your Use Case

1. **Open Dashboard**: Navigate to `http://localhost:3000/dashboard/settings`
2. **Select Method**: Choose "Rate Limited" from Rotation Method dropdown
3. **Set Max Requests**: Enter `30` in "Max Requests Per Minute" field
4. **Set Window**: Enter `60` in "Time Window (seconds)" field
5. **Save**: Click "Save Configuration" button

**Result**: Each of your 100 proxies will handle exactly 30 requests per minute!

## Type Safety

The dashboard TypeScript types have been updated to include:
- `"rate-limited" | "rate_limited"` in the rotation method type
- `rate_limited?: { max_requests_per_minute: number, window_seconds: number }` in rotation settings

## Automatic Initialization

When you select "Rate Limited" for the first time:
- The `rate_limited` object is automatically initialized
- Default values (30 requests, 60 seconds) are set
- You can immediately adjust them

## Validation

The dashboard enforces:
- **Min Value**: Both fields require values >= 1
- **Type Safety**: TypeScript ensures correct data types
- **Required Fields**: Both fields are required when rate-limited is selected

## Real-Time Updates

Changes take effect immediately after clicking "Save Configuration":
- Settings are sent to the API
- Rota core reloads settings
- New proxy selections use the updated configuration

## API Integration

The dashboard uses the same API endpoint as manual configuration:
- **GET** `/api/v1/settings` - Fetches current settings
- **PUT** `/api/v1/settings` - Updates settings

The dashboard automatically:
- Fetches settings on page load
- Validates input before saving
- Shows success/error toasts
- Handles errors gracefully

## Troubleshooting

### Issue: Rate Limited option not showing

**Solution**: 
- Ensure you're running the latest version of the dashboard
- Clear browser cache
- Check that the API is returning the updated settings structure

### Issue: Settings not saving

**Solution**:
- Check browser console for errors
- Verify API is accessible
- Ensure authentication token is valid
- Check network tab for API response

### Issue: Default values not appearing

**Solution**:
- The dashboard initializes defaults when you select "Rate Limited"
- If values are missing, manually enter: 30 for max requests, 60 for window

## Summary

✅ **Fully Configurable**: All settings available in dashboard UI  
✅ **User-Friendly**: Clear labels and descriptions  
✅ **Type-Safe**: TypeScript ensures correct data types  
✅ **Auto-Initialize**: Defaults set automatically  
✅ **Real-Time**: Changes apply immediately  
✅ **No API Required**: Complete configuration via UI  

You can configure your rate-limited rotation entirely through the dashboard - no need to use curl or API calls!

