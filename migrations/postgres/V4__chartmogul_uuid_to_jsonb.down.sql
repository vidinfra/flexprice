-- Rollback: Change chartmogul_uuid column on plans table from jsonb back to varchar
-- Data loss is intentional

BEGIN;

DROP INDEX IF EXISTS idx_plans_chartmogul_uuid;

ALTER TABLE public.plans
    DROP COLUMN IF EXISTS chartmogul_uuid;

ALTER TABLE public.plans
    ADD COLUMN chartmogul_uuid varchar(255);

CREATE INDEX idx_plans_chartmogul_uuid
    ON public.plans (chartmogul_uuid)
    WHERE chartmogul_uuid IS NOT NULL;

COMMIT;
