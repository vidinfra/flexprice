-- Migration: Change chartmogul_uuid column on plans table from varchar to jsonb
-- Data loss is intentional

BEGIN;

-- 1. Drop index if it exists
DROP INDEX IF EXISTS idx_plans_chartmogul_uuid;

-- 2. Drop the old column entirely
ALTER TABLE public.plans
    DROP COLUMN IF EXISTS chartmogul_uuid;

-- 3. Recreate column with new semantic type
ALTER TABLE public.plans
    ADD COLUMN chartmogul_uuid jsonb;

-- 4. (Optional) JSON index for future access
CREATE INDEX idx_plans_chartmogul_uuid
    ON public.plans USING gin (chartmogul_uuid);

COMMIT;
