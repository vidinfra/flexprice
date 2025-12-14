-- Rollback: Remove ChartMogul UUID columns from entities
-- This migration removes the dedicated ChartMogul UUID columns

-- Before removing, migrate data back to metadata (if needed for rollback)
-- For customers
UPDATE public.customers
SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('chartmogul_customer_uuid', chartmogul_uuid)
WHERE chartmogul_uuid IS NOT NULL;

-- For plans
UPDATE public.plans
SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('chartmogul_plan_uuid', chartmogul_uuid)
WHERE chartmogul_uuid IS NOT NULL;

-- For subscriptions
UPDATE public.subscriptions
SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('chartmogul_subscription_invoice_uuid', chartmogul_invoice_uuid)
WHERE chartmogul_invoice_uuid IS NOT NULL;

-- For invoices
UPDATE public.invoices
SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('chartmogul_invoice_uuid', chartmogul_uuid)
WHERE chartmogul_uuid IS NOT NULL;

-- Drop indexes
DROP INDEX IF EXISTS public.idx_customers_chartmogul_uuid;
DROP INDEX IF EXISTS public.idx_plans_chartmogul_uuid;
DROP INDEX IF EXISTS public.idx_subscriptions_chartmogul_invoice_uuid;
DROP INDEX IF EXISTS public.idx_invoices_chartmogul_uuid;

-- Remove columns
ALTER TABLE public.customers DROP COLUMN IF EXISTS chartmogul_uuid;
ALTER TABLE public.plans DROP COLUMN IF EXISTS chartmogul_uuid;
ALTER TABLE public.subscriptions DROP COLUMN IF EXISTS chartmogul_invoice_uuid;
ALTER TABLE public.invoices DROP COLUMN IF EXISTS chartmogul_uuid;
