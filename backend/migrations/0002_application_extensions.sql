CREATE TABLE document_workflows (
    id BIGINT NOT NULL PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    original_filename VARCHAR(500) NOT NULL,
    storage_path VARCHAR(1000) NOT NULL,
    source_type VARCHAR(20) NOT NULL,
    mime_type VARCHAR(150) NOT NULL,
    order_type VARCHAR(50),
    order_year_be INT NOT NULL DEFAULT 0,
    status VARCHAR(30) NOT NULL DEFAULT 'NEEDS_REVIEW',
    order_no VARCHAR(100),
    committee VARCHAR(500),
    signer_name VARCHAR(500),
    signer_position VARCHAR(255),
    responsibilities TEXT,
    additional_refs TEXT,
    confidentiality VARCHAR(30),
    quality_profile VARCHAR(30),
    page_count INT NOT NULL DEFAULT 0,
    person_count INT NOT NULL DEFAULT 0,
    file_size_bytes BIGINT NOT NULL DEFAULT 0,
    file_hash VARCHAR(64) NOT NULL,
    retention_until DATETIME(3),
    import_source VARCHAR(30) NOT NULL DEFAULT 'UPLOAD',
    processing_stage VARCHAR(50),
    processing_started_at DATETIME(3),
    processing_finished_at DATETIME(3),
    processing_duration_ms BIGINT NOT NULL DEFAULT 0,
    review_notes TEXT,
    issued_date DATETIME(3),
    effective_date DATETIME(3),
    expiry_date DATETIME(3),
    confidence FLOAT NOT NULL DEFAULT 0,
    imported_by VARCHAR(255),
    confirmed_by VARCHAR(255),
    confirmed_at DATETIME(3),
    ocr_provider VARCHAR(100),
    ocr_text LONGTEXT,
    ocr_error LONGTEXT,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY ux_document_workflows_file_hash (file_hash),
    KEY ix_document_workflows_status (status),
    KEY ix_document_workflows_expiry_date (expiry_date),
    CONSTRAINT fk_document_workflows_document FOREIGN KEY (id) REFERENCES documents (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE extracted_fields (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT NOT NULL,
    field_key VARCHAR(100) NOT NULL,
    field_label VARCHAR(255) NOT NULL,
    field_type VARCHAR(50),
    value TEXT,
    source_text TEXT,
    confidence FLOAT NOT NULL DEFAULT 0,
    page_no INT NOT NULL DEFAULT 0,
    bounding_box JSON,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    verified_by VARCHAR(255),
    verified_at DATETIME(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_extracted_fields_document_id (document_id),
    CONSTRAINT fk_extracted_fields_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE ocr_pages (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT NOT NULL,
    page_no INT NOT NULL DEFAULT 0,
    image_width INT NOT NULL DEFAULT 0,
    image_height INT NOT NULL DEFAULT 0,
    image_angle FLOAT NOT NULL DEFAULT 0,
    raw_text LONGTEXT,
    status VARCHAR(30),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_ocr_pages_document_id (document_id),
    CONSTRAINT fk_ocr_pages_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE ocr_lines (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT NOT NULL,
    ocr_page_id BIGINT NOT NULL,
    page_no INT NOT NULL DEFAULT 0,
    line_index INT NOT NULL DEFAULT 0,
    text LONGTEXT NOT NULL,
    confidence FLOAT NOT NULL DEFAULT 0,
    bounding_box JSON,
    raw_json JSON,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_ocr_lines_document_id (document_id),
    KEY ix_ocr_lines_ocr_page_id (ocr_page_id),
    CONSTRAINT fk_ocr_lines_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE,
    CONSTRAINT fk_ocr_lines_page FOREIGN KEY (ocr_page_id) REFERENCES ocr_pages (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE appointments (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT NOT NULL,
    directory_user_id BIGINT,
    full_name VARCHAR(500) NOT NULL,
    position VARCHAR(255),
    department VARCHAR(500),
    committee_role VARCHAR(255),
    responsibilities TEXT,
    name_match_method VARCHAR(50),
    name_match_score FLOAT NOT NULL DEFAULT 0,
    confidence FLOAT NOT NULL DEFAULT 0,
    page_no INT NOT NULL DEFAULT 0,
    bounding_box JSON,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    verified_by VARCHAR(255),
    verified_at DATETIME(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_appointments_document_id (document_id),
    KEY ix_appointments_directory_user_id (directory_user_id),
    CONSTRAINT fk_appointments_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE,
    CONSTRAINT fk_appointments_person FOREIGN KEY (directory_user_id) REFERENCES directory_people (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE document_references (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT NOT NULL,
    reference_type VARCHAR(100),
    reference_text TEXT,
    page_no INT NOT NULL DEFAULT 0,
    bounding_box JSON,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_document_references_document_id (document_id),
    CONSTRAINT fk_document_references_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE document_revisions (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT NOT NULL,
    field_key VARCHAR(100) NOT NULL,
    old_value TEXT,
    new_value TEXT,
    changed_by VARCHAR(255) NOT NULL,
    changed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY ix_document_revisions_document_id (document_id),
    CONSTRAINT fk_document_revisions_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE expiry_notifications (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT NOT NULL,
    appointment_id BIGINT,
    notice_type VARCHAR(20) NOT NULL,
    due_at DATETIME(3) NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',
    sent_at DATETIME(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_expiry_notifications_document_id (document_id),
    KEY ix_expiry_notifications_appointment_id (appointment_id),
    KEY ix_expiry_notifications_status (status),
    CONSTRAINT fk_expiry_notifications_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE,
    CONSTRAINT fk_expiry_notifications_appointment FOREIGN KEY (appointment_id) REFERENCES appointments (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE application_audit_logs (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT,
    user_id VARCHAR(255) NOT NULL,
    action VARCHAR(100) NOT NULL,
    details TEXT,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY ix_application_audit_logs_document_id (document_id),
    CONSTRAINT fk_application_audit_logs_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE user_sync_runs (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    started_at DATETIME(3) NOT NULL,
    completed_at DATETIME(3),
    status VARCHAR(30) NOT NULL,
    fetched INT NOT NULL DEFAULT 0,
    created INT NOT NULL DEFAULT 0,
    updated INT NOT NULL DEFAULT 0,
    unchanged INT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_user_sync_runs_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE directory_person_profiles (
    person_id BIGINT NOT NULL PRIMARY KEY,
    department VARCHAR(255),
    faculty VARCHAR(255),
    source VARCHAR(20) NOT NULL DEFAULT 'SYNCED',
    created_by VARCHAR(255),
    source_updated_at VARCHAR(50),
    raw_payload JSON,
    fingerprint CHAR(64),
    KEY ix_directory_person_profiles_source (source),
    CONSTRAINT fk_directory_person_profiles_person FOREIGN KEY (person_id) REFERENCES directory_people (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE person_name_aliases (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    normalized_name VARCHAR(255) NOT NULL,
    directory_person_id BIGINT NOT NULL,
    source_text VARCHAR(500),
    created_by VARCHAR(255),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY ux_person_name_aliases_name (normalized_name),
    KEY ix_person_name_aliases_person (directory_person_id),
    CONSTRAINT fk_person_name_aliases_person FOREIGN KEY (directory_person_id) REFERENCES directory_people (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
