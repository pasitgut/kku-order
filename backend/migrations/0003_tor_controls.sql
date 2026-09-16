ALTER TABLE documents ADD COLUMN IF NOT EXISTS retention_until DATETIME(3) NULL;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS import_source VARCHAR(30) NOT NULL DEFAULT 'UPLOAD';
ALTER TABLE documents ADD KEY ix_documents_retention_until (retention_until);
