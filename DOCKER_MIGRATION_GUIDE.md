# ChartMogul UUID Migration - Docker Quick Start

## Your Setup
- **Database**: PostgreSQL in Docker (docker-compose.yml)
- **User**: flexprice
- **Password**: flexprice123
- **Database**: flexprice
- **Port**: 5432 (mapped to localhost)

---

## 🚀 Run Migration (Recommended Method)

```bash
cd /Users/tenbyte/Documents/TenByte/Services/flexprice

# 1. Start postgres
docker compose up -d postgres
sleep 5

# 2. Backup (optional but recommended)
docker compose exec -T postgres pg_dump -U flexprice -d flexprice -F c > backup_$(date +%Y%m%d_%H%M%S).dump

# 3. Run migration
docker compose exec -T postgres psql -U flexprice -d flexprice < migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql

# 4. Verify
docker compose exec -T postgres psql -U flexprice -d flexprice -c "
SELECT table_name, column_name 
FROM information_schema.columns 
WHERE column_name LIKE '%chartmogul%';
"
```

**Expected Output**: Should show 4 columns:
- `customers.chartmogul_uuid`
- `plans.chartmogul_uuid`
- `subscriptions.chartmogul_invoice_uuid`
- `invoices.chartmogul_uuid`

---

## ✅ Verify Data Migration

```bash
docker compose exec -T postgres psql -U flexprice -d flexprice -c "
SELECT 
  'customers' as table, 
  COUNT(*) as total,
  COUNT(chartmogul_uuid) as migrated 
FROM customers
UNION ALL 
SELECT 'plans', COUNT(*), COUNT(chartmogul_uuid) FROM plans
UNION ALL 
SELECT 'subscriptions', COUNT(*), COUNT(chartmogul_invoice_uuid) FROM subscriptions
UNION ALL 
SELECT 'invoices', COUNT(*), COUNT(chartmogul_uuid) FROM invoices;
"
```

---

## 🔄 Rollback (If Something Goes Wrong)

```bash
cd /Users/tenbyte/Documents/TenByte/Services/flexprice

# Rollback the migration
docker compose exec -T postgres psql -U flexprice -d flexprice < migrations/postgres/V3__add_chartmogul_uuid_columns.down.sql

# Verify rollback (should return no rows)
docker compose exec -T postgres psql -U flexprice -d flexprice -c "
SELECT column_name 
FROM information_schema.columns 
WHERE column_name LIKE '%chartmogul%';
"
```

---

## 📝 Alternative: Using psql from Your Mac

If you have `psql` installed on your Mac:

```bash
cd /Users/tenbyte/Documents/TenByte/Services/flexprice

export PGPASSWORD=flexprice123

# Backup
pg_dump -h localhost -p 5432 -U flexprice -d flexprice -F c -f backup_$(date +%Y%m%d_%H%M%S).dump

# Run migration
psql -h localhost -p 5432 -U flexprice -d flexprice -f migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql

# Verify
psql -h localhost -p 5432 -U flexprice -d flexprice -c "SELECT COUNT(*) FROM customers WHERE chartmogul_uuid IS NOT NULL;"

unset PGPASSWORD
```

---

## 🔍 Troubleshooting

### Container not running?
```bash
docker compose ps
docker compose up -d postgres
docker compose logs postgres
```

### Check if migration already ran?
```bash
docker compose exec postgres psql -U flexprice -d flexprice -c "\d customers"
# Look for "chartmogul_uuid" column
```

### Interactive psql session?
```bash
docker compose exec postgres psql -U flexprice -d flexprice
# Then run queries manually
```

---

## ⚠️ Important Notes

1. **The migration is safe** - it only adds new columns, doesn't remove anything
2. **Data is preserved** - existing metadata is copied to new columns
3. **Reversible** - you can rollback if needed
4. **No downtime required** - can run while app is running (but restart app after)

---

## 📱 After Migration - Restart Your App

```bash
# If running via Docker Compose
docker compose restart

# If running locally with air/go run
# Just restart your terminal/process

# Verify app started successfully
docker compose logs -f --tail=100
```

---

## ✨ That's It!

Once migration is complete, your ChartMogul integration will use the new dedicated columns instead of metadata. All future creates/updates will automatically use these columns.
