# ChartMogul UUID Migration Summary

## Overview
This document summarizes the migration from storing ChartMogul UUIDs in entity metadata to dedicated database columns for the Flexprice backend.

## Migration Date
Performed on: 2024

## Affected Entities
The following entities now have dedicated ChartMogul UUID columns:

1. **Customers** - `chartmogul_uuid` column
2. **Plans** - `chartmogul_uuid` column  
3. **Subscriptions** - `chartmogul_invoice_uuid` column (stores the ChartMogul invoice UUID used to create the subscription)
4. **Invoices** - `chartmogul_uuid` column

## Database Changes

### Migration Files
- **Upgrade**: `/migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql`
- **Rollback**: `/migrations/postgres/V3__add_chartmogul_uuid_columns.down.sql`

### Schema Changes
The upgrade migration performs the following:

1. **Adds new columns** to the respective tables:
   - `customers.chartmogul_uuid` (VARCHAR 255)
   - `plans.chartmogul_uuid` (VARCHAR 255)
   - `subscriptions.chartmogul_invoice_uuid` (VARCHAR 255)
   - `invoices.chartmogul_uuid` (VARCHAR 255)

2. **Migrates existing data** from metadata to the new columns:
   - `metadata->>'chartmogul_customer_uuid'` → `customers.chartmogul_uuid`
   - `metadata->>'chartmogul_plan_uuid'` → `plans.chartmogul_uuid`
   - `metadata->>'chartmogul_subscription_invoice_uuid'` → `subscriptions.chartmogul_invoice_uuid`
   - `metadata->>'chartmogul_invoice_uuid'` → `invoices.chartmogul_uuid`

3. **Creates indexes** for efficient lookups:
   - `idx_customers_chartmogul_uuid` on `customers(chartmogul_uuid)`
   - `idx_plans_chartmogul_uuid` on `plans(chartmogul_uuid)`
   - `idx_subscriptions_chartmogul_invoice_uuid` on `subscriptions(chartmogul_invoice_uuid)`
   - `idx_invoices_chartmogul_uuid` on `invoices(chartmogul_uuid)`

4. **Adds column comments** for documentation

The rollback migration reverses all these changes and moves data back to metadata.

## Code Changes

### Domain Models Updated
1. **`/internal/domain/customer/model.go`**
   - Added: `ChartMogulUUID *string` field

2. **`/internal/domain/plan/model.go`**
   - Added: `ChartMogulUUID *string` field

3. **`/internal/domain/subscription/model.go`**
   - Added: `ChartMogulInvoiceUUID *string` field

4. **`/internal/domain/invoice/model.go`**
   - Added: `ChartMogulUUID *string` field

### Service Layer Updated

#### Customer Service (`/internal/service/customer.go`)
- **CreateCustomer**: Now stores ChartMogul UUID in `customer.ChartMogulUUID` instead of metadata
- **UpdateCustomer**: Now reads ChartMogul UUID from `customer.ChartMogulUUID` instead of metadata
- **DeleteCustomer**: Now reads ChartMogul UUID from `customer.ChartMogulUUID` instead of metadata

#### Plan Service (`/internal/service/plan.go`)
- **CreatePlan**: Now stores ChartMogul UUID in `plan.ChartMogulUUID` instead of metadata
- **UpdatePlan**: Now reads ChartMogul UUID from `plan.ChartMogulUUID` instead of metadata
- **DeletePlan**: Now reads ChartMogul UUID from `plan.ChartMogulUUID` instead of metadata

#### Subscription Service (`/internal/service/subscription.go`)
- **CreateSubscription**: 
  - Now reads customer's ChartMogul UUID from `customer.ChartMogulUUID`
  - Now reads plan's ChartMogul UUID from `plan.ChartMogulUUID`
  - Now stores ChartMogul invoice UUID in `subscription.ChartMogulInvoiceUUID` instead of metadata

#### Invoice Service (`/internal/service/invoice.go`)
- **FinalizeInvoice**: 
  - Now reads customer's ChartMogul UUID from `customer.ChartMogulUUID`
  - Now stores ChartMogul UUID in `invoice.ChartMogulUUID` instead of metadata
- **UpdatePaymentStatus**: Now reads ChartMogul UUID from `invoice.ChartMogulUUID` instead of metadata

## Benefits of This Migration

1. **Performance**: 
   - Direct column access is faster than JSON path queries
   - Indexes on UUID columns enable efficient lookups
   - Reduced metadata JSON parsing overhead

2. **Data Integrity**:
   - Column-level constraints can be enforced
   - Type safety at the database level
   - Clearer schema definition

3. **Query Efficiency**:
   - Simpler SQL queries without JSON operators
   - Better query optimization by the database
   - Easier to write and maintain queries

4. **Maintainability**:
   - Explicit schema makes the integration more visible
   - Easier to understand the data model
   - Better documentation through column comments

## Rollback Procedure

If you need to rollback this migration:

1. **Stop the application** to prevent new data from being written
2. **Run the rollback migration**:
   ```bash
   # Using your migration tool
   migrate -path migrations/postgres -database "postgres://..." down 1
   ```
3. **Verify data migration**: Check that all UUIDs were moved back to metadata
4. **Restart the application** with the previous code version

## Testing Recommendations

Before deploying to production:

1. **Test the upgrade migration** in a staging environment
2. **Verify data migration** - ensure all UUIDs are correctly moved to new columns
3. **Test all ChartMogul sync operations**:
   - Customer creation, update, deletion
   - Plan creation, update, deletion
   - Subscription creation
   - Invoice finalization and payment updates
4. **Test the rollback migration** to ensure it works correctly
5. **Verify indexes** are created and being used

## Deployment Steps

1. **Backup the database** before running the migration
2. **Run the upgrade migration** during a maintenance window
3. **Deploy the updated code** with the new ChartMogul UUID handling
4. **Monitor logs** for any ChartMogul sync errors
5. **Verify** that new entities get ChartMogul UUIDs stored in the correct columns

## Notes

- The metadata columns still exist and can still store other custom data
- The old metadata keys (`chartmogul_customer_uuid`, etc.) are NOT automatically removed from metadata
- Future cleanup of metadata can be done if desired, but is not required
- All new ChartMogul syncs will use the dedicated columns going forward
