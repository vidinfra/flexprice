# ChartMogul UUID Migration - Testing Checklist

## Pre-Migration Validation

### Database Backup
- [ ] Full database backup completed
- [ ] Backup verified and can be restored
- [ ] Backup stored in secure location

### Code Review
- [ ] All domain models updated with new ChartMogul UUID fields
- [ ] All service methods updated to use new columns instead of metadata
- [ ] No remaining references to old metadata keys in code
- [ ] Code compiles successfully

### Migration Scripts
- [ ] Upgrade migration script reviewed
- [ ] Rollback migration script reviewed
- [ ] Migration tested in local environment
- [ ] Migration tested in staging environment

## Migration Execution

### Upgrade Migration
- [ ] Application stopped/scaled down
- [ ] Migration executed: `V3__add_chartmogul_uuid_columns.up.sql`
- [ ] Migration completed without errors
- [ ] All new columns created successfully
- [ ] All indexes created successfully

### Data Migration Verification
- [ ] Count of customers with ChartMogul UUIDs in metadata matches count in new column
- [ ] Count of plans with ChartMogul UUIDs in metadata matches count in new column
- [ ] Count of subscriptions with ChartMogul UUIDs in metadata matches count in new column
- [ ] Count of invoices with ChartMogul UUIDs in metadata matches count in new column

#### SQL Validation Queries
```sql
-- Verify customer migration
SELECT 
  COUNT(*) FILTER (WHERE metadata->>'chartmogul_customer_uuid' IS NOT NULL) as metadata_count,
  COUNT(*) FILTER (WHERE chartmogul_uuid IS NOT NULL) as column_count
FROM customers;

-- Verify plan migration
SELECT 
  COUNT(*) FILTER (WHERE metadata->>'chartmogul_plan_uuid' IS NOT NULL) as metadata_count,
  COUNT(*) FILTER (WHERE chartmogul_uuid IS NOT NULL) as column_count
FROM plans;

-- Verify subscription migration
SELECT 
  COUNT(*) FILTER (WHERE metadata->>'chartmogul_subscription_invoice_uuid' IS NOT NULL) as metadata_count,
  COUNT(*) FILTER (WHERE chartmogul_invoice_uuid IS NOT NULL) as column_count
FROM subscriptions;

-- Verify invoice migration
SELECT 
  COUNT(*) FILTER (WHERE metadata->>'chartmogul_invoice_uuid' IS NOT NULL) as metadata_count,
  COUNT(*) FILTER (WHERE chartmogul_uuid IS NOT NULL) as column_count
FROM invoices;
```

### Index Verification
- [ ] Index `idx_customers_chartmogul_uuid` exists and is valid
- [ ] Index `idx_plans_chartmogul_uuid` exists and is valid
- [ ] Index `idx_subscriptions_chartmogul_invoice_uuid` exists and is valid
- [ ] Index `idx_invoices_chartmogul_uuid` exists and is valid

#### SQL Validation Query
```sql
-- Check all indexes exist
SELECT 
  schemaname, 
  tablename, 
  indexname, 
  indexdef 
FROM pg_indexes 
WHERE indexname IN (
  'idx_customers_chartmogul_uuid',
  'idx_plans_chartmogul_uuid', 
  'idx_subscriptions_chartmogul_invoice_uuid',
  'idx_invoices_chartmogul_uuid'
);
```

## Post-Migration Testing

### Application Deployment
- [ ] Updated code deployed to staging/production
- [ ] Application started successfully
- [ ] No startup errors in logs
- [ ] Health checks passing

### Customer ChartMogul Sync Testing
- [ ] Create a new customer
- [ ] Verify ChartMogul customer created
- [ ] Verify ChartMogul UUID stored in `customers.chartmogul_uuid` column
- [ ] Update the customer
- [ ] Verify ChartMogul customer updated using UUID from column
- [ ] Delete the customer (or test customer)
- [ ] Verify ChartMogul customer deleted using UUID from column

### Plan ChartMogul Sync Testing
- [ ] Create a new plan
- [ ] Verify ChartMogul plan created
- [ ] Verify ChartMogul UUID stored in `plans.chartmogul_uuid` column
- [ ] Update the plan
- [ ] Verify ChartMogul plan updated using UUID from column
- [ ] Delete the plan (or test plan)
- [ ] Verify ChartMogul plan deleted using UUID from column

### Subscription ChartMogul Sync Testing
- [ ] Create a new subscription
- [ ] Verify ChartMogul subscription created via invoice import
- [ ] Verify ChartMogul invoice UUID stored in `subscriptions.chartmogul_invoice_uuid` column
- [ ] Verify customer and plan UUIDs read correctly from their respective columns

### Invoice ChartMogul Sync Testing
- [ ] Finalize a subscription invoice
- [ ] Verify ChartMogul invoice created
- [ ] Verify ChartMogul UUID stored in `invoices.chartmogul_uuid` column
- [ ] Update invoice payment status to succeeded
- [ ] Verify ChartMogul transaction created using UUID from column

### Existing Data Testing
- [ ] Test operations on existing customers (created before migration)
- [ ] Test operations on existing plans (created before migration)
- [ ] Test operations on existing subscriptions (created before migration)
- [ ] Test operations on existing invoices (created before migration)
- [ ] Verify all operations use ChartMogul UUIDs from the new columns

### Monitoring and Logs
- [ ] Monitor application logs for ChartMogul sync errors
- [ ] Monitor database logs for query errors
- [ ] Check for any warnings about missing ChartMogul UUIDs
- [ ] Verify performance metrics are normal

### Performance Testing
- [ ] Query performance on customers by ChartMogul UUID
- [ ] Query performance on plans by ChartMogul UUID
- [ ] Query performance on subscriptions by ChartMogul invoice UUID
- [ ] Query performance on invoices by ChartMogul UUID
- [ ] Compare with previous metadata-based query performance

## Rollback Testing (Optional, in Staging Only)

### Execute Rollback
- [ ] Stop the application
- [ ] Run rollback migration: `V3__add_chartmogul_uuid_columns.down.sql`
- [ ] Verify data moved back to metadata
- [ ] Verify columns dropped
- [ ] Verify indexes dropped
- [ ] Deploy old code version
- [ ] Test ChartMogul sync operations with old code

### Rollback Verification Queries
```sql
-- After rollback, verify data is back in metadata
SELECT 
  COUNT(*) FILTER (WHERE metadata->>'chartmogul_customer_uuid' IS NOT NULL) as customer_count
FROM customers;

SELECT 
  COUNT(*) FILTER (WHERE metadata->>'chartmogul_plan_uuid' IS NOT NULL) as plan_count
FROM plans;

SELECT 
  COUNT(*) FILTER (WHERE metadata->>'chartmogul_subscription_invoice_uuid' IS NOT NULL) as subscription_count
FROM subscriptions;

SELECT 
  COUNT(*) FILTER (WHERE metadata->>'chartmogul_invoice_uuid' IS NOT NULL) as invoice_count
FROM invoices;
```

## Production Deployment Checklist

### Pre-Deployment
- [ ] All staging tests passed
- [ ] Database backup completed
- [ ] Rollback plan documented and tested
- [ ] Deployment window scheduled (low traffic period recommended)
- [ ] Team notified of deployment
- [ ] Monitoring alerts configured

### During Deployment
- [ ] Scale down application instances (optional, for zero-downtime)
- [ ] Run migration
- [ ] Verify migration success
- [ ] Deploy new code
- [ ] Scale up application instances
- [ ] Verify application health

### Post-Deployment
- [ ] Monitor application logs for 1 hour
- [ ] Monitor ChartMogul sync operations
- [ ] Monitor database performance
- [ ] Verify new customers/plans/subscriptions/invoices created successfully
- [ ] Verify ChartMogul UUIDs stored in new columns
- [ ] Document any issues encountered

## Sign-Off

### Staging Environment
- [ ] Migration successful
- [ ] All tests passed
- [ ] Approved by: _________________ Date: _________

### Production Environment
- [ ] Migration successful
- [ ] All tests passed
- [ ] Approved by: _________________ Date: _________

## Notes and Issues

Record any issues, warnings, or observations during testing:

```
Issue/Observation:
___________________________________________________________________________
___________________________________________________________________________
___________________________________________________________________________

Resolution:
___________________________________________________________________________
___________________________________________________________________________
___________________________________________________________________________
```

## Rollback Decision Criteria

Rollback immediately if:
- Migration fails to complete
- Data corruption detected
- Critical ChartMogul sync failures
- Application cannot start
- Performance degradation >50%

Monitor and consider rollback if:
- Intermittent ChartMogul sync errors
- Performance degradation 20-50%
- Unexpected behavior in ChartMogul integration

## Contact Information

- **Migration Owner**: _________________
- **Database Admin**: _________________
- **On-Call Engineer**: _________________
- **ChartMogul Support**: support@chartmogul.com
