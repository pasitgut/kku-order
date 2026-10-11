ALTER TABLE document_workflows ADD COLUMN IF NOT EXISTS cancellation_reason TEXT NULL;
ALTER TABLE document_workflows ADD COLUMN IF NOT EXISTS cancelled_by VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE document_workflows ADD COLUMN IF NOT EXISTS cancelled_at DATETIME(3) NULL;
