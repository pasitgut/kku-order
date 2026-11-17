CREATE TABLE IF NOT EXISTS directory_people (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    external_id BIGINT UNIQUE,
    prefix VARCHAR(50),
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE,
    phone VARCHAR(20) UNIQUE,
    position_title VARCHAR(255),
    status VARCHAR(50),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    synced_at TIMESTAMP NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS documents (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    original_filename VARCHAR(500) NOT NULL,
    drive_web_view_link VARCHAR(2048) NOT NULL DEFAULT '',
    storage_path VARCHAR(1000) NOT NULL,
    extraction_json_path VARCHAR(1000) NOT NULL DEFAULT '',
    pending_extraction_json_path VARCHAR(1000) NOT NULL DEFAULT '',
    content_state VARCHAR(20) NOT NULL DEFAULT 'LEGACY',
    source_type VARCHAR(20) NOT NULL,
    mime_type VARCHAR(150) NOT NULL,
    order_type VARCHAR(50),
    issuing_unit VARCHAR(255) NOT NULL DEFAULT '',
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
    extraction_attempts INT NOT NULL DEFAULT 0,
    next_attempt_at DATETIME NULL,
    extraction_last_status INT NOT NULL DEFAULT 0,
    processing_duration_ms BIGINT NOT NULL DEFAULT 0,
    review_notes TEXT,
    issued_date DATETIME(3),
    effective_date DATETIME(3),
    expiry_date DATETIME(3),
    confidence FLOAT NOT NULL DEFAULT 0,
    imported_by VARCHAR(255),
    confirmed_by VARCHAR(255),
    confirmed_at DATETIME(3),
    cancellation_reason TEXT NULL,
    replaced_by_order_no VARCHAR(100) NOT NULL DEFAULT '',
    cancelled_by VARCHAR(255) NOT NULL DEFAULT '',
    cancelled_at DATETIME(3) NULL,
    ocr_provider VARCHAR(100),
    ocr_error LONGTEXT,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY ux_documents_file_hash (file_hash),
    KEY ix_documents_status (status),
    KEY ix_documents_expiry_date (expiry_date),
    KEY ix_documents_next_attempt (processing_stage, next_attempt_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS extracted_fields (
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

CREATE TABLE IF NOT EXISTS appointments (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT NOT NULL,
    directory_user_id BIGINT,
    full_name VARCHAR(500) NOT NULL,
    position VARCHAR(255),
    department VARCHAR(500),
    committee_role VARCHAR(255),
    responsibilities TEXT,
    extra_duties TEXT NULL,
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

CREATE TABLE IF NOT EXISTS document_references (
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

CREATE TABLE IF NOT EXISTS document_revisions (
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

CREATE TABLE IF NOT EXISTS expiry_notifications (
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

CREATE TABLE IF NOT EXISTS application_audit_logs (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT,
    user_id VARCHAR(255) NOT NULL,
    action VARCHAR(100) NOT NULL,
    details TEXT,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY ix_application_audit_logs_document_id (document_id),
    CONSTRAINT fk_application_audit_logs_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_sync_runs (
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

CREATE TABLE IF NOT EXISTS directory_person_profiles (
    person_id BIGINT NOT NULL PRIMARY KEY,
    department VARCHAR(255),
    faculty VARCHAR(255),
    source VARCHAR(20) NOT NULL DEFAULT 'SYNCED',
    created_by VARCHAR(255),
    source_updated_at VARCHAR(50),
    raw_payload JSON,
    local_overrides JSON NULL,
    fingerprint CHAR(64),
    KEY ix_directory_person_profiles_source (source),
    CONSTRAINT fk_directory_person_profiles_person FOREIGN KEY (person_id) REFERENCES directory_people (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS person_name_aliases (
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

CREATE TABLE IF NOT EXISTS app_settings (
    setting_key VARCHAR(100) NOT NULL PRIMARY KEY,
    value JSON NOT NULL,
    updated_by VARCHAR(255) NULL,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_profiles (
    user_id VARCHAR(255) NOT NULL PRIMARY KEY,
    photo_path VARCHAR(500) NOT NULL DEFAULT '',
    photo_updated_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS drive_sync_runs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    root_id VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    root_resource_key VARCHAR(255) NOT NULL DEFAULT '',
    folder_name VARCHAR(500) NOT NULL DEFAULT '',
    status VARCHAR(30) NOT NULL,
    trigger_type VARCHAR(20) NOT NULL DEFAULT 'SCHEDULED',
    started_at DATETIME(3) NOT NULL,
    finished_at DATETIME(3) NULL,
    retry_at DATETIME(3) NULL,
    last_error TEXT NOT NULL,
    KEY ix_drive_runs_root (root_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS drive_sync_folders (
    run_id BIGINT UNSIGNED NOT NULL,
    folder_id VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    resource_key VARCHAR(255) NOT NULL DEFAULT '',
    drive_id VARCHAR(255) NOT NULL DEFAULT '',
    page_token TEXT NOT NULL,
    status VARCHAR(30) NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    retry_at DATETIME(3) NULL,
    last_error TEXT NOT NULL,
    PRIMARY KEY (run_id, folder_id),
    KEY ix_drive_folders_pending (run_id, status, retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS drive_sync_items (
    run_id BIGINT UNSIGNED NOT NULL,
    file_id VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    file_name VARCHAR(500) NOT NULL,
    metadata JSON NOT NULL,
    status VARCHAR(30) NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    retry_at DATETIME(3) NULL,
    last_error TEXT NOT NULL,
    PRIMARY KEY (run_id, file_id),
    KEY ix_drive_items_pending (run_id, status, retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS drive_imported_files (
    file_id VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    file_hash CHAR(64) NOT NULL,
    imported_at DATETIME(3) NOT NULL,
    web_view_link VARCHAR(2048) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS position_holders (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    position_key VARCHAR(255) NOT NULL,
    position_title VARCHAR(255) NOT NULL,
    directory_person_id BIGINT NOT NULL,
    starts_on DATE NULL,
    ends_on DATE NULL,
    source VARCHAR(20) NOT NULL,
    created_by VARCHAR(255),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_position_holders_key (position_key),
    KEY ix_position_holders_person (directory_person_id),
    CONSTRAINT fk_position_holders_person FOREIGN KEY (directory_person_id) REFERENCES directory_people (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS directory_person_revisions (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    directory_person_id BIGINT NOT NULL,
    changes JSON NOT NULL,
    changed_by VARCHAR(255) NOT NULL,
    changed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY ix_directory_person_revisions_person_changed (directory_person_id, changed_at),
    CONSTRAINT fk_directory_person_revisions_person FOREIGN KEY (directory_person_id) REFERENCES directory_people (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
