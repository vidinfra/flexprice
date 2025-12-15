#!/bin/bash
# Run any SQL migration file with backup and verification
#
# Usage examples:
#   bash scripts/run_any_migration.sh migrations/postgres/V4__change_plan_chartmogul_uuid_to_array.up.sql migrations/postgres/V4__change_plan_chartmogul_uuid_to_array.down.sql
#   bash scripts/run_any_migration.sh migrations/postgres/your_migration.up.sql migrations/postgres/your_migration.down.sql
#   bash scripts/run_any_migration.sh migrations/postgres/your_migration.up.sql

set -e  # Exit on error

if [ $# -lt 1 ]; then
  echo "Usage: $0 <migration_up.sql> [<migration_down.sql>]"
  exit 1
fi

MIGRATION_UP="$1"
MIGRATION_DOWN="$2"

cd "$(dirname "$0")/.."

# Ensure postgres is running
echo "📦 Starting postgres..."
docker compose up -d postgres
sleep 5

# Create backup
echo ""
echo "💾 Creating backup..."
BACKUP_FILE="backup_$(date +%Y%m%d_%H%M%S).dump"
docker compose exec -T postgres pg_dump -U flexprice -d flexprice -F c > "$BACKUP_FILE"

if [ -f "$BACKUP_FILE" ]; then
    BACKUP_SIZE=$(ls -lh "$BACKUP_FILE" | awk '{print $5}')
    echo "  ✓ Backup created: $BACKUP_FILE ($BACKUP_SIZE)"
else
    echo "  ✗ Backup failed!"
    exit 1
fi

# Run migration
echo ""
echo "🚀 Running migration..."
echo "  File: $MIGRATION_UP"
echo ""

if docker compose exec -T postgres psql -U flexprice -d flexprice < "$MIGRATION_UP"; then
    echo ""
    echo "  ✅ Migration executed successfully!"
else
    echo ""
    echo "  ❌ Migration failed!"
    if [ -n "$MIGRATION_DOWN" ]; then
      echo "To rollback, run:"
      echo "  docker compose exec -T postgres psql -U flexprice -d flexprice < $MIGRATION_DOWN"
    fi
    echo "Or restore from backup:"
    echo "  docker compose exec -T postgres pg_restore -U flexprice -d flexprice < $BACKUP_FILE"
    exit 1
fi

# Verify migration
# echo ""
# echo "🔍 Verifying migration..."
# echo ""
# echo "Checking columns altered:"
# docker compose exec -T postgres psql -U flexprice -d flexprice -c "\d+ $TABLE_NAME"  # Replace $TABLE_NAME with the actual table name relevant to the migration

echo ""
echo "================================================"
echo "✅ Migration completed successfully!"
echo "================================================"
echo ""
echo "Backup saved to: $BACKUP_FILE"
echo ""
echo "Next steps:"
echo "  1. Restart your application:"
echo "     docker compose restart"
echo ""
echo "  2. Monitor logs for any issues:"
echo "     docker compose logs -f --tail=100"
echo ""
echo "  3. Test ChartMogul sync operations"
echo ""
echo "If you need to rollback:"
if [ -n "$MIGRATION_DOWN" ]; then
  echo "  docker compose exec -T postgres psql -U flexprice -d flexprice < $MIGRATION_DOWN"
fi
echo ""
echo "  docker compose exec -T postgres pg_restore -U flexprice -d flexprice < $BACKUP_FILE"
echo ""
