-- Add sync_config column to connections table
ALTER TABLE connections ADD COLUMN sync_config JSONB;

