# Quick Migration Commands Reference

## Local Environment (Docker Compose)

### Run Migration (Recommended - Using docker compose exec)
```bash
# Navigate to project root
cd /Users/tenbyte/Documents/TenByte/Services/flexprice

# Ensure postgres is running
docker compose up -d postgres

# Run the migration
docker compose exec -T postgres psql -U flexprice -d flexprice < migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql
```

### Run Migration (Alternative - Using psql from host)
```bash
# From project root with password
export PGPASSWORD=flexprice123
psql -h localhost -p 5432 -U flexprice -d flexprice -f migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql
unset PGPASSWORD
```

### Verify Migration
```bash
# Using docker compose
docker compose exec -T postgres psql -U flexprice -d flexprice -c "
SELECT table_name, column_name FROM information_schema.columns 
WHERE column_name LIKE '%chartmogul%' ORDER BY table_name;
"

# Or using psql from host
export PGPASSWORD=flexprice123
psql -h localhost -p 5432 -U flexprice -d flexprice -c "SELECT COUNT(*) FROM customers WHERE chartmogul_uuid IS NOT NULL;"
unset PGPASSWORD
```

### Rollback (if needed)
```bash
docker compose exec -T postgres psql -U flexprice -d flexprice < migrations/postgres/V3__add_chartmogul_uuid_columns.down.sql
```

---

## Staging Environment

### Run Migration
```bash
# Replace with your actual staging credentials
psql postgresql://user:password@staging-db-host:5432/flexprice_staging -f /Users/tenbyte/Documents/TenByte/Services/flexprice/migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql
```

### Verify Migration
```bash
psql postgresql://user:password@staging-db-host:5432/flexprice_staging -c "
SELECT 'customers' as table, COUNT(*) FILTER (WHERE chartmogul_uuid IS NOT NULL) as migrated FROM customers
UNION ALL SELECT 'plans', COUNT(*) FILTER (WHERE chartmogul_uuid IS NOT NULL) FROM plans
UNION ALL SELECT 'subscriptions', COUNT(*) FILTER (WHERE chartmogul_invoice_uuid IS NOT NULL) FROM subscriptions
UNION ALL SELECT 'invoices', COUNT(*) FILTER (WHERE chartmogul_uuid IS NOT NULL) FROM invoices;
"
```

### Rollback (if needed)
```bash
psql postgresql://user:password@staging-db-host:5432/flexprice_staging -f /Users/tenbyte/Documents/TenByte/Services/flexprice/migrations/postgres/V3__add_chartmogul_uuid_columns.down.sql
```

---

## Using Environment Variables

```bash
# Set once, use multiple times
export DATABASE_URL="postgresql://postgres:password@localhost:5432/flexprice_local"

# Run migration
psql $DATABASE_URL -f migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql

# Verify
psql $DATABASE_URL -c "SELECT COUNT(*) FROM customers WHERE chartmogul_uuid IS NOT NULL;"

# Rollback
psql $DATABASE_URL -f migrations/postgres/V3__add_chartmogul_uuid_columns.down.sql
```

---

## Complete Flow - Local Docker (Copy & Paste)

```bash
# Navigate to project root
cd /Users/tenbyte/Documents/TenByte/Services/flexprice

# Ensure postgres is running
docker compose up -d postgres && sleep 5

# Create backup
docker compose exec -T postgres pg_dump -U flexprice -d flexprice -F c > backup_$(date +%Y%m%d_%H%M%S).dump

# Run migration
docker compose exec -T postgres psql -U flexprice -d flexprice < migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql

# Verify - check indexes
docker compose exec -T postgres psql -U flexprice -d flexprice -c "SELECT indexname FROM pg_indexes WHERE indexname LIKE '%chartmogul%';"

# Verify - check data migration
docker compose exec -T postgres psql -U flexprice -d flexprice -c "
SELECT 'customers' as table, COUNT(*) FILTER (WHERE chartmogul_uuid IS NOT NULL) as migrated FROM customers
UNION ALL SELECT 'plans', COUNT(*) FILTER (WHERE chartmogul_uuid IS NOT NULL) FROM plans
UNION ALL SELECT 'subscriptions', COUNT(*) FILTER (WHERE chartmogul_invoice_uuid IS NOT NULL) FROM subscriptions
UNION ALL SELECT 'invoices', COUNT(*) FILTER (WHERE chartmogul_uuid IS NOT NULL) FROM invoices;
"

echo "✅ Migration complete!"
```

### Staging (with SSH tunnel)
```bash
# Create SSH tunnel
ssh -N -L 5433:your-db-host:5432 your-bastion-host &

# Set DB URL (through tunnel)
export DATABASE_URL="postgresql://user:password@localhost:5433/flexprice_staging"

# Backup
pg_dump $DATABASE_URL -F c -f backup_staging_$(date +%Y%m%d_%H%M%S).dump

# Run migration
psql $DATABASE_URL -f migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql

# Verify
psql $DATABASE_URL -c "SELECT table_name, column_name FROM information_schema.columns WHERE column_name LIKE '%chartmogul%';"

# Kill SSH tunnel when done
killall ssh
```
