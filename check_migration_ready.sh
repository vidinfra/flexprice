#!/bin/bash
# Pre-migration validation script

set -e  # Exit on error

echo "================================================"
echo "ChartMogul UUID Migration - Pre-flight Check"
echo "================================================"
echo ""

cd /var/www/html/flexprice

# Check 1: Migration file exists
echo "✓ Checking migration file exists..."
if [ -f "migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql" ]; then
    echo "  ✓ Migration file found"
else
    echo "  ✗ Migration file NOT found!"
    exit 1
fi

# Check 2: Rollback file exists
echo "✓ Checking rollback file exists..."
if [ -f "migrations/postgres/V3__add_chartmogul_uuid_columns.down.sql" ]; then
    echo "  ✓ Rollback file found"
else
    echo "  ✗ Rollback file NOT found!"
    exit 1
fi

# Check 3: Docker compose is available
echo "✓ Checking docker compose..."
if command -v docker &> /dev/null; then
    echo "  ✓ Docker is installed"
else
    echo "  ✗ Docker not found!"
    exit 1
fi

# Check 4: Start postgres if not running
echo "✓ Starting postgres container..."
docker compose up -d postgres
sleep 3

# Check 5: Verify postgres is responsive
echo "✓ Checking postgres connection..."
if docker compose exec -T postgres psql -U flexprice -d flexprice -c "SELECT 1;" &> /dev/null; then
    echo "  ✓ Postgres is responsive"
else
    echo "  ✗ Cannot connect to postgres!"
    echo "  Waiting 5 more seconds..."
    sleep 5
    if docker compose exec -T postgres psql -U flexprice -d flexprice -c "SELECT 1;" &> /dev/null; then
        echo "  ✓ Postgres is now responsive"
    else
        echo "  ✗ Still cannot connect. Please check docker logs."
        exit 1
    fi
fi

# Check 6: Verify tables exist
echo "✓ Checking required tables exist..."
TABLES=$(docker compose exec -T postgres psql -U flexprice -d flexprice -t -c "
SELECT COUNT(*) FROM information_schema.tables 
WHERE table_schema = 'public' 
AND table_name IN ('customers', 'plans', 'subscriptions', 'invoices');
")

if [ "$TABLES" -eq 4 ]; then
    echo "  ✓ All required tables exist"
else
    echo "  ⚠ Warning: Expected 4 tables, found $TABLES"
    echo "  This might be okay if database is fresh"
fi

# Check 7: Check if migration already ran
echo "✓ Checking if migration already applied..."
EXISTING_COLUMNS=$(docker compose exec -T postgres psql -U flexprice -d flexprice -t -c "
SELECT COUNT(*) FROM information_schema.columns 
WHERE table_schema = 'public' 
AND column_name LIKE '%chartmogul%';
")

if [ "$EXISTING_COLUMNS" -gt 0 ]; then
    echo "  ⚠ WARNING: Found $EXISTING_COLUMNS ChartMogul columns already!"
    echo "  Migration may have already been applied."
    echo "  Do you want to continue anyway? (y/N)"
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        echo "  Aborting."
        exit 0
    fi
else
    echo "  ✓ No existing ChartMogul columns found (good)"
fi

# Check 8: Count existing ChartMogul UUIDs in metadata
echo "✓ Checking existing ChartMogul data in metadata..."
CUSTOMER_UUIDS=$(docker compose exec -T postgres psql -U flexprice -d flexprice -t -c "
SELECT COUNT(*) FROM customers WHERE metadata->>'chartmogul_customer_uuid' IS NOT NULL;
" | tr -d ' ')

PLAN_UUIDS=$(docker compose exec -T postgres psql -U flexprice -d flexprice -t -c "
SELECT COUNT(*) FROM plans WHERE metadata->>'chartmogul_plan_uuid' IS NOT NULL;
" | tr -d ' ')

SUBSCRIPTION_UUIDS=$(docker compose exec -T postgres psql -U flexprice -d flexprice -t -c "
SELECT COUNT(*) FROM subscriptions WHERE metadata->>'chartmogul_subscription_invoice_uuid' IS NOT NULL;
" | tr -d ' ')

INVOICE_UUIDS=$(docker compose exec -T postgres psql -U flexprice -d flexprice -t -c "
SELECT COUNT(*) FROM invoices WHERE metadata->>'chartmogul_invoice_uuid' IS NOT NULL;
" | tr -d ' ')

echo "  Customers with ChartMogul UUID: $CUSTOMER_UUIDS"
echo "  Plans with ChartMogul UUID: $PLAN_UUIDS"
echo "  Subscriptions with ChartMogul UUID: $SUBSCRIPTION_UUIDS"
echo "  Invoices with ChartMogul UUID: $INVOICE_UUIDS"

echo ""
echo "================================================"
echo "✅ Pre-flight check PASSED!"
echo "================================================"
echo ""
echo "You can now run the migration with:"
echo ""
echo "  ./run_migration.sh"
echo ""
echo "Or manually:"
echo ""
echo "  # Backup"
echo "  docker compose exec -T postgres pg_dump -U flexprice -d flexprice -F c > backup_\$(date +%Y%m%d_%H%M%S).dump"
echo ""
echo "  # Run migration"
echo "  docker compose exec -T postgres psql -U flexprice -d flexprice < migrations/postgres/V3__add_chartmogul_uuid_columns.up.sql"
echo ""
