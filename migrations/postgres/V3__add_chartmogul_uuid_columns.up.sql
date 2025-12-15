-- Migration: Add ChartMogul UUID columns to entities
-- This migration adds dedicated columns for storing ChartMogul UUIDs instead of using metadata

-- Add chartmogul_uuid column to customers table
ALTER TABLE public.customers
ADD COLUMN chartmogul_uuid VARCHAR(255);

-- Add index for faster ChartMogul UUID lookups
CREATE INDEX idx_customers_chartmogul_uuid ON public.customers(chartmogul_uuid) WHERE chartmogul_uuid IS NOT NULL;

-- Add chartmogul_uuid column to plans table
ALTER TABLE public.plans
ADD COLUMN chartmogul_uuid VARCHAR(255);

-- Add index for faster ChartMogul UUID lookups
CREATE INDEX idx_plans_chartmogul_uuid ON public.plans(chartmogul_uuid) WHERE chartmogul_uuid IS NOT NULL;

-- Add chartmogul_invoice_uuid column to subscriptions table
-- Note: Subscriptions are created via invoice import in ChartMogul, so we store the invoice UUID
ALTER TABLE public.subscriptions
ADD COLUMN chartmogul_invoice_uuid VARCHAR(255);

-- Add index for faster ChartMogul UUID lookups
CREATE INDEX idx_subscriptions_chartmogul_invoice_uuid ON public.subscriptions(chartmogul_invoice_uuid) WHERE chartmogul_invoice_uuid IS NOT NULL;

-- Add chartmogul_uuid column to invoices table
ALTER TABLE public.invoices
ADD COLUMN chartmogul_uuid VARCHAR(255);

-- Add index for faster ChartMogul UUID lookups
CREATE INDEX idx_invoices_chartmogul_uuid ON public.invoices(chartmogul_uuid) WHERE chartmogul_uuid IS NOT NULL;

-- Migrate existing ChartMogul UUIDs from metadata to dedicated columns
-- For customers
UPDATE public.customers
SET chartmogul_uuid = metadata->>'chartmogul_customer_uuid'
WHERE metadata IS NOT NULL 
  AND metadata->>'chartmogul_customer_uuid' IS NOT NULL;

-- For plans
UPDATE public.plans
SET chartmogul_uuid = metadata->>'chartmogul_plan_uuid'
WHERE metadata IS NOT NULL 
  AND metadata->>'chartmogul_plan_uuid' IS NOT NULL;

-- For subscriptions
UPDATE public.subscriptions
SET chartmogul_invoice_uuid = metadata->>'chartmogul_subscription_invoice_uuid'
WHERE metadata IS NOT NULL 
  AND metadata->>'chartmogul_subscription_invoice_uuid' IS NOT NULL;

-- For invoices
UPDATE public.invoices
SET chartmogul_uuid = metadata->>'chartmogul_invoice_uuid'
WHERE metadata IS NOT NULL 
  AND metadata->>'chartmogul_invoice_uuid' IS NOT NULL;

-- Add comments for documentation
COMMENT ON COLUMN public.customers.chartmogul_uuid IS 'ChartMogul customer UUID for analytics sync';
COMMENT ON COLUMN public.plans.chartmogul_uuid IS 'ChartMogul plan UUID for analytics sync';
COMMENT ON COLUMN public.subscriptions.chartmogul_invoice_uuid IS 'ChartMogul invoice UUID used to create this subscription';
COMMENT ON COLUMN public.invoices.chartmogul_uuid IS 'ChartMogul invoice UUID for analytics sync';
