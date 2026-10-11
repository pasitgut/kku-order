ALTER TABLE document_workflows ADD COLUMN IF NOT EXISTS extraction_attempts INT NOT NULL DEFAULT 0;
ALTER TABLE document_workflows ADD COLUMN IF NOT EXISTS next_attempt_at DATETIME NULL;
ALTER TABLE document_workflows ADD COLUMN IF NOT EXISTS extraction_last_status INT NOT NULL DEFAULT 0;
ALTER TABLE document_workflows ADD KEY ix_document_workflows_next_attempt (processing_stage, next_attempt_at);
