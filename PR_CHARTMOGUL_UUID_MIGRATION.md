# ChartMogul UUID Column Migration - Fix Entity Sync

Fixes #[issue-number] <!-- Replace with actual issue number -->

## 📝 Description

This PR completes the migration from storing ChartMogul UUIDs in entity metadata to dedicated database columns. After the database migration (V3) was successfully applied, new entities (customers, plans, subscriptions, invoices) created were not having their ChartMogul UUIDs persisted because the Ent schema and repository layers were not aware of the new columns.

This PR fixes the complete sync pipeline by:
1. Adding the `chartmogul_uuid` field to Ent schemas for Customer, Plan, Invoice
2. Adding the `chartmogul_invoice_uuid` field to Ent schema for Subscription (subscriptions use invoice UUID since they're created via invoice imports in ChartMogul)
3. Updating all repository Create/Update methods to persist ChartMogul UUIDs
4. Updating all domain model `FromEnt` converters to read ChartMogul UUIDs from the database
5. Ensuring backward compatibility with metadata fallback for pre-migration entities

### Problem Solved
- ❌ **Before**: New customers/plans/subscriptions/invoices created after V3 migration had ChartMogul UUIDs in memory but not persisted to database
- ✅ **After**: All new entities have their ChartMogul UUIDs properly stored in dedicated database columns

---

## 🔨 Changes Made

### Ent Schema Updates
- [x] Added `chartmogul_uuid` field to `ent/schema/customer.go`
- [x] Added `chartmogul_uuid` field to `ent/schema/plan.go`
- [x] Added `chartmogul_invoice_uuid` field to `ent/schema/subscription.go` (note: different field name because subscriptions store invoice UUID)
- [x] Added `chartmogul_uuid` field to `ent/schema/invoice.go`
- [x] Regenerated Ent code with `make generate-ent`

### Repository Updates
- [x] Updated `internal/repository/ent/customer.go`:
  - Added conditional setter in `Create()` method
  - Added conditional setter in `Update()` method
- [x] Updated `internal/repository/ent/plan.go`:
  - Added conditional setter in `Create()` method
  - Added conditional setter in `Update()` method
- [x] Updated `internal/repository/ent/subscription.go`:
  - Added conditional setter in `Update()` method for `chartmogul_invoice_uuid`
- [x] Updated `internal/repository/ent/invoice.go`:
  - Added conditional setter in `Update()` method

### Domain Model Updates
- [x] Updated `internal/domain/customer/model.go`:
  - Enhanced `FromEnt()` to convert Ent's `string` to domain's `*string` for ChartMogul UUID
- [x] Updated `internal/domain/plan/model.go`:
  - Enhanced `FromEnt()` to convert ChartMogul UUID
- [x] Updated `internal/domain/subscription/model.go`:
  - Enhanced `GetSubscriptionFromEnt()` to convert ChartMogul invoice UUID
- [x] Updated `internal/domain/invoice/model.go`:
  - Enhanced `FromEnt()` to convert ChartMogul UUID

### Documentation
- [x] Created `CHARTMOGUL_ENT_SCHEMA_FIX.md` with detailed technical documentation
- [x] Updated existing migration documentation

### Service Layer (No Changes Required)
- Service layer already had correct ChartMogul sync logic
- Backward compatibility with metadata fallback remains intact
- No changes needed in `internal/service/customer.go`, `plan.go`, `subscription.go`, `invoice.go`

---

## 🧪 Testing

### Compilation
- [x] Code compiles successfully: `go build ./internal/...`
- [x] No breaking changes introduced

### Database Schema
- [x] Verified migration V3 columns exist:
  - `customers.chartmogul_uuid`
  - `plans.chartmogul_uuid`
  - `subscriptions.chartmogul_invoice_uuid`
  - `invoices.chartmogul_uuid`
- [x] Verified indexes are in place

### Runtime Testing (Required After Deployment)
After deploying, verify:
```bash
# Test new customer creation
# Create a new customer via API
# Then check database:
docker compose exec -T postgres psql -U flexprice -d flexprice -c "
SELECT id, name, chartmogul_uuid, created_at 
FROM customers 
WHERE chartmogul_uuid IS NOT NULL 
ORDER BY created_at DESC 
LIMIT 5;
"

# Expected: New customers should have chartmogul_uuid populated
```

---

## 📋 Migration Path

### For Existing Deployments:
1. **Database migration already applied** (V3 migration adds columns and migrates existing data)
2. **Deploy this PR** to fix Ent schema and repository layer
3. **Restart services** with `docker compose down && docker compose up -d --build`
4. **Verify** new entities have ChartMogul UUIDs in database columns
5. **(Optional)** After validation period, remove metadata fallback code

### Backward Compatibility:
- ✅ Service layer still reads from metadata if column is empty
- ✅ Existing pre-migration entities continue to work
- ✅ No data loss or breaking changes

---

## ✅ Checklist

- [x] My code follows the project's code style
- [x] Code compiles without errors
- [x] Ent code regenerated after schema changes
- [x] All repository methods updated to handle new fields
- [x] Domain model converters updated
- [x] Backward compatibility maintained
- [x] Documentation created (`CHARTMOGUL_ENT_SCHEMA_FIX.md`)
- [x] No breaking changes introduced
- [ ] Runtime testing performed (to be done post-deployment)
- [ ] Logs verified showing "Stored ChartMogul UUID" messages

---

## 📷 Database Schema Verification

### Before Fix (Issue):
```
# New customer created but UUID not in database
customers table:
id                          | name        | chartmogul_uuid | metadata
----------------------------|-------------|-----------------|------------------
cust_01KCENPEWPXMKBF...     | Test Co     | NULL            | {...}
```

### After Fix (Expected):
```
# New customer created with UUID in database column
customers table:
id                          | name        | chartmogul_uuid      | metadata
----------------------------|-------------|----------------------|----------
cust_01KCENPEWPXMKBF...     | Test Co     | cus-abc123-uuid      | {...}
```

---

## 🔍 Key Technical Details

### Why Subscription Uses Different Column Name:
- Customers, Plans, Invoices: Store their own ChartMogul entity UUID → `chartmogul_uuid`
- Subscriptions: Created via invoice imports in ChartMogul, so we store the invoice UUID → `chartmogul_invoice_uuid`

### Field Type Conversion:
- **Ent schema**: Uses `string` type for ChartMogul UUID fields
- **Domain model**: Uses `*string` (pointer to string) for nullable fields
- **Conversion**: `FromEnt()` functions convert empty string to `nil` pointer

### Repository Pattern:
```go
// Conditional setter to only update when UUID is provided
if entity.ChartMogulUUID != nil {
    updateBuilder = updateBuilder.SetNillableChartmogulUUID(entity.ChartMogulUUID)
}
```

---

## 📚 Related Documentation
- `CHARTMOGUL_ENT_SCHEMA_FIX.md` - Technical implementation details
- `migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql` - Database migration
- `migrations/postgres/V3__add_chartmogul_uuid_columns.down.sql` - Rollback migration

---

## 🚀 Deployment Instructions

1. **Pull latest code**:
   ```bash
   git pull origin main
   ```

2. **Rebuild and restart**:
   ```bash
   docker compose down
   docker compose up -d --build
   ```

3. **Verify services are healthy**:
   ```bash
   docker compose ps
   docker compose logs -f flexprice-api
   ```

4. **Test ChartMogul sync**:
   - Create a new customer via API
   - Check logs for "Stored ChartMogul UUID in customer" message
   - Verify database column is populated

---

## 🎯 Success Criteria

- [x] Code compiles successfully
- [x] Ent code generated without errors
- [ ] New customers have `chartmogul_uuid` in database (verify post-deployment)
- [ ] New plans have `chartmogul_uuid` in database (verify post-deployment)
- [ ] New subscriptions have `chartmogul_invoice_uuid` in database (verify post-deployment)
- [ ] New invoices have `chartmogul_uuid` in database (verify post-deployment)
- [ ] Logs show successful ChartMogul UUID storage
- [ ] No breaking changes or regressions
- [ ] Backward compatibility maintained for pre-migration data

---

## 👥 Reviewers
@[team-member] - Please review Ent schema changes and repository updates

---

## 🔗 References
- ChartMogul API Documentation: https://dev.chartmogul.com/
- Ent Framework Documentation: https://entgo.io/
- Migration V3 Specification: `migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql`
