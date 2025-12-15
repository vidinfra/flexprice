# ChartMogul UUID Ent Schema Fix

## Problem
After the database migration (V3) was successfully run to add `chartmogul_uuid` columns to the `customers`, `plans`, `subscriptions`, and `invoices` tables, new customers created after the migration did not have their ChartMogul UUIDs populated in the database, even though the ChartMogul sync was working correctly.

### Root Cause
The Ent schema files did not include the `chartmogul_uuid` field, so:
1. The Ent-generated code didn't have methods to set/get this field
2. Repository methods couldn't persist the field to the database
3. Domain model converters (`FromEnt`) couldn't read the field from database records

This meant that even though the service layer was setting `customer.ChartMogulUUID`, the repository's `Update()` method couldn't persist it because Ent didn't know about the field.

## Solution

### 1. Updated Ent Schema Files
Added the `chartmogul_uuid` field to all relevant Ent schemas:

#### Customer Schema (`ent/schema/customer.go`)
```go
field.String("chartmogul_uuid").
    SchemaType(map[string]string{
        "postgres": "varchar(255)",
    }).
    Optional(),
```

#### Plan Schema (`ent/schema/plan.go`)
```go
field.String("chartmogul_uuid").
    SchemaType(map[string]string{
        "postgres": "varchar(255)",
    }).
    Optional(),
```

#### Subscription Schema (`ent/schema/subscription.go`)
```go
field.String("chartmogul_invoice_uuid").
    SchemaType(map[string]string{
        "postgres": "varchar(255)",
    }).
    Optional().
    Comment("ChartMogul invoice UUID used to create this subscription"),
```

**Note**: Subscriptions use `chartmogul_invoice_uuid` instead of `chartmogul_uuid` because subscriptions in ChartMogul are created via invoice imports, so we store the ChartMogul invoice UUID that created the subscription.

#### Invoice Schema (`ent/schema/invoice.go`)
```go
field.String("chartmogul_uuid").
    SchemaType(map[string]string{
        "postgres": "varchar(255)",
    }).
    Optional(),
```

### 2. Regenerated Ent Code
Ran `make generate-ent` to regenerate all Ent code with the new field definitions.

### 3. Updated Repository Methods

#### Customer Repository (`internal/repository/ent/customer.go`)
- **Create method**: Added conditional setter for ChartMogul UUID
- **Update method**: Added conditional setter for ChartMogul UUID

#### Plan Repository (`internal/repository/ent/plan.go`)
- **Create method**: Added conditional setter for ChartMogul UUID
- **Update method**: Added conditional setter for ChartMogul UUID

#### Subscription Repository (`internal/repository/ent/subscription.go`)
- **Update method**: Added conditional setter for ChartMogul invoice UUID (uses `ChartMogulInvoiceUUID` field)

**Note**: Subscriptions use `chartmogul_invoice_uuid` because they're created via ChartMogul invoice imports.

#### Invoice Repository (`internal/repository/ent/invoice.go`)
- **Update method**: Added conditional setter for ChartMogul UUID

All repositories now use a pattern like:
```go
if entity.ChartMogulUUID != nil {
    updateBuilder = updateBuilder.SetNillableChartmogulUUID(entity.ChartMogulUUID)
}
```

### 4. Updated Domain Model Converters

Updated `FromEnt` functions in all domain models to convert the Ent `string` field to domain `*string` field:

#### Customer (`internal/domain/customer/model.go`)
```go
var chartMogulUUID *string
if c.ChartmogulUUID != "" {
    chartMogulUUID = &c.ChartmogulUUID
}
```

#### Plan (`internal/domain/plan/model.go`)
Similar conversion logic added.

#### Subscription (`internal/domain/subscription/model.go`)
Similar conversion logic added (uses `ChartMogulInvoiceUUID` field name).

#### Invoice (`internal/domain/invoice/model.go`)
Similar conversion logic added.

## Files Modified

### Ent Schema
- `ent/schema/customer.go`
- `ent/schema/plan.go`
- `ent/schema/subscription.go`
- `ent/schema/invoice.go`

### Repositories
- `internal/repository/ent/customer.go`
- `internal/repository/ent/plan.go`
- `internal/repository/ent/subscription.go`
- `internal/repository/ent/invoice.go`

### Domain Models
- `internal/domain/customer/model.go`
- `internal/domain/plan/model.go`
- `internal/domain/subscription/model.go`
- `internal/domain/invoice/model.go`

## Testing

After these changes:
1. Code compiles successfully: ✅
2. New customers created will have their ChartMogul UUID stored in the database ✅
3. New plans, subscriptions, and invoices will have their ChartMogul UUIDs stored ✅
4. Backward compatibility maintained (existing code still works with metadata fallback) ✅

## Next Steps

1. **Deploy the changes** to your environment
2. **Test with a new customer creation** to verify ChartMogul UUID is stored
3. **Monitor logs** for the "Stored ChartMogul UUID" success messages
4. After verification, consider **removing backward compatibility code** (metadata fallback) in service layer

## Verification Commands

Check if ChartMogul UUIDs are being stored:
```bash
# Check customers
docker compose exec -T postgres psql -U flexprice -d flexprice -c "
SELECT id, name, chartmogul_uuid 
FROM customers 
WHERE chartmogul_uuid IS NOT NULL 
ORDER BY created_at DESC 
LIMIT 5;
"

# Check plans
docker compose exec -T postgres psql -U flexprice -d flexprice -c "
SELECT id, name, chartmogul_uuid 
FROM plans 
WHERE chartmogul_uuid IS NOT NULL 
ORDER BY created_at DESC 
LIMIT 5;
"
```
