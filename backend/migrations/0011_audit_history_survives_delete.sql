ALTER TABLE application_audit_logs DROP FOREIGN KEY IF EXISTS fk_application_audit_logs_document;
ALTER TABLE application_audit_logs ADD CONSTRAINT IF NOT EXISTS fk_application_audit_logs_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE SET NULL;
