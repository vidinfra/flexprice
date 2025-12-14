# ChartMogul UUID Migration - Backward Compatibility Fix

## Issue

After updating the code to use the new `chartmogul_uuid` columns, you're seeing warnings like:

```
ChartMogul customer UUID not found, skipping subscription sync
```

This happens because:
1. The migration **hasn't been run yet** - the new columns don't exist in the database
2. The code was updated to read from the new columns (`customer.ChartMogulUUID`)
3. Existing customers have their ChartMogul UUIDs stored in `metadata` only
4. The code couldn't find the UUID, so ChartMogul sync was skipped

## Solution

Added **backward compatibility** to the code so it works both **before** and **after** the migration:

### Updated Services

1. **Subscription Service** (`internal/service/subscription.go`)
2. **Invoice Service** (`internal/service/invoice.go`)

### How It Works

The code now follows this priority:

```go
var customerUUID string
if customer.ChartMogulUUID != nil && *customer.ChartMogulUUID != "" {
    // ✅ Prefer the new column (after migration)
    customerUUID = *customer.ChartMogulUUID
} else if customer.Metadata != nil {
    // ✅ Fall back to metadata (before migration)
    if uuid, exists := customer.Metadata["chartmogul_customer_uuid"]; exists {
        customerUUID = uuid
    }
}
```

### Migration Timeline

| Stage | Column Exists? | Metadata Has UUID? | What Happens |
|-------|---------------|-------------------|--------------|
| **Before Migration** | ❌ No | ✅ Yes | Reads from metadata (fallback) |
| **After Migration** | ✅ Yes | ✅ Yes | Reads from column (preferred) |
| **New Customers** | ✅ Yes | ❌ No | Reads from column only |

## Benefits

✅ **Zero Downtime**: Code works before and after migration  
✅ **Safe Deployment**: Can deploy code changes before running migration  
✅ **Gradual Rollout**: Run migration on your schedule  
✅ **No Sync Loss**: ChartMogul sync continues working throughout  

## Deployment Sequence

You can now deploy in either order:

### Option 1: Code First (Recommended)
```bash
1. Deploy updated code (with backward compatibility) ✅ Done
2. Test - ChartMogul sync still works via metadata
3. Run migration when ready
4. ChartMogul sync automatically uses new columns
```

### Option 2: Migration First
```bash
1. Run migration (adds columns, migrates data)
2. Deploy updated code
3. ChartMogul sync uses new columns immediately
```

## Testing

### Before Migration
```bash
# Create a customer - should sync to ChartMogul
# Check: ChartMogul UUID stored in metadata

# Create a subscription - should sync to ChartMogul
# No warnings in logs

# Create an invoice - should sync to ChartMogul
# No warnings in logs
```

### After Migration
```bash
# Existing customers - UUID in both column and metadata
# New customers - UUID only in column

# All ChartMogul operations work seamlessly
```

## Removing Backward Compatibility (Future)

After the migration has been running successfully for some time (e.g., 1-2 weeks), you can:

1. Remove the metadata fallback code
2. Keep only the column-based reads
3. Optionally clean up ChartMogul UUIDs from metadata

This is optional and not urgent - the fallback code is minimal and doesn't impact performance.

## Summary

The warning you saw was expected - it indicated the code was looking for UUIDs in the new column that didn't exist yet. With the backward compatibility fix, the code now:

- ✅ Works before migration (reads from metadata)
- ✅ Works after migration (reads from new column)
- ✅ Continues to sync to ChartMogul seamlessly
- ✅ Allows flexible deployment timing

You can now proceed with the migration at your convenience!
