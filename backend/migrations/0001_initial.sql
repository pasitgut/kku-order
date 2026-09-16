CREATE TABLE IF NOT EXISTS documents (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    original_filename VARCHAR(255) NOT NULL,
    storage_path VARCHAR(500) NOT NULL,
    source_type VARCHAR(20) NOT NULL,
    mime_type VARCHAR(150) NOT NULL DEFAULT 'application/octet-stream',
    order_type VARCHAR(50) NULL,
    order_year_be INT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'PROCESSING',
    order_no VARCHAR(100) NULL,
    committee VARCHAR(500) NULL,
    signer_name VARCHAR(500) NULL,
    responsibilities LONGTEXT NULL,
    additional_refs LONGTEXT NULL,
    confidentiality VARCHAR(30) NULL,
    quality_profile VARCHAR(30) NULL,
    page_count INT NOT NULL DEFAULT 0,
    person_count INT NOT NULL DEFAULT 0,
    file_size_bytes BIGINT NOT NULL DEFAULT 0,
    file_hash CHAR(64) NOT NULL,
    processing_stage VARCHAR(50) NULL,
    processing_started_at DATETIME(3) NULL,
    processing_finished_at DATETIME(3) NULL,
    processing_duration_ms BIGINT NOT NULL DEFAULT 0,
    review_notes LONGTEXT NULL,
    issued_date DATETIME(3) NULL,
    effective_date DATETIME(3) NULL,
    expiry_date DATETIME(3) NULL,
    confidence FLOAT NOT NULL DEFAULT 0,
    imported_by VARCHAR(255) NULL,
    confirmed_by VARCHAR(255) NULL,
    confirmed_at DATETIME(3) NULL,
    ocr_provider VARCHAR(100) NULL,
    ocr_text LONGTEXT NULL,
    ocr_error LONGTEXT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY ux_documents_file_hash (file_hash),
    KEY ix_documents_status (status),
    KEY ix_documents_order_no (order_no),
    KEY ix_documents_order_year_be (order_year_be),
    KEY ix_documents_confidentiality (confidentiality),
    KEY ix_documents_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS extracted_fields (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    field_key VARCHAR(100) NOT NULL,
    field_label VARCHAR(255) NOT NULL,
    field_type VARCHAR(50) NULL,
    value LONGTEXT NULL,
    source_text LONGTEXT NULL,
    confidence FLOAT NOT NULL DEFAULT 0,
    page_no INT NOT NULL DEFAULT 0,
    bounding_box_json JSON NULL,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    verified_by VARCHAR(255) NULL,
    verified_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_extracted_fields_document_id (document_id),
    KEY ix_extracted_fields_field_key (field_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ocr_pages (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    page_no INT NOT NULL,
    image_width INT NOT NULL DEFAULT 0,
    image_height INT NOT NULL DEFAULT 0,
    image_angle FLOAT NOT NULL DEFAULT 0,
    raw_text LONGTEXT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'COMPLETED',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY ux_ocr_pages_document_page (document_id, page_no),
    KEY ix_ocr_pages_document_id (document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ocr_lines (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    ocr_page_id BIGINT UNSIGNED NOT NULL,
    page_no INT NOT NULL,
    line_index INT NOT NULL,
    text LONGTEXT NOT NULL,
    confidence FLOAT NOT NULL DEFAULT 0,
    bounding_box_json JSON NULL,
    raw_json JSON NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_ocr_lines_document_id (document_id),
    KEY ix_ocr_lines_ocr_page_id (ocr_page_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS appointments (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    directory_user_id BIGINT UNSIGNED NULL,
    full_name VARCHAR(500) NOT NULL,
    position VARCHAR(255) NULL,
    committee_role VARCHAR(255) NULL,
    responsibilities LONGTEXT NULL,
    name_match_method VARCHAR(50) NULL,
    name_match_score FLOAT NOT NULL DEFAULT 0,
    confidence FLOAT NOT NULL DEFAULT 0,
    page_no INT NOT NULL DEFAULT 0,
    bounding_box_json JSON NULL,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    verified_by VARCHAR(255) NULL,
    verified_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_appointments_document_id (document_id),
    KEY ix_appointments_directory_user_id (directory_user_id),
    KEY ix_appointments_full_name (full_name(191))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS document_references (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    reference_type VARCHAR(100) NULL,
    reference_text LONGTEXT NULL,
    page_no INT NOT NULL DEFAULT 0,
    bounding_box_json JSON NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_document_references_document_id (document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS document_revisions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    field_key VARCHAR(100) NOT NULL,
    old_value LONGTEXT NULL,
    new_value LONGTEXT NULL,
    changed_by VARCHAR(255) NOT NULL,
    changed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY ix_document_revisions_document_id (document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NULL,
    user_id VARCHAR(255) NOT NULL,
    action VARCHAR(100) NOT NULL,
    details LONGTEXT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY ix_audit_logs_document_id (document_id),
    KEY ix_audit_logs_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS directory_users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    external_id VARCHAR(255) NOT NULL,
    username VARCHAR(255) NULL,
    full_name VARCHAR(500) NULL,
    first_name VARCHAR(255) NULL,
    last_name VARCHAR(255) NULL,
    email VARCHAR(255) NULL,
    phone VARCHAR(100) NULL,
    department VARCHAR(255) NULL,
    faculty VARCHAR(255) NULL,
    job_title VARCHAR(255) NULL,
    role VARCHAR(100) NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    raw_payload JSON NULL,
    fingerprint CHAR(64) NULL,
    last_seen_at DATETIME(3) NULL,
    synced_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY ux_directory_users_external_id (external_id),
    KEY ix_directory_users_username (username),
    KEY ix_directory_users_email (email),
    KEY ix_directory_users_fingerprint (fingerprint)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_sync_runs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    started_at DATETIME(3) NOT NULL,
    completed_at DATETIME(3) NULL,
    status VARCHAR(30) NOT NULL,
    fetched INT NOT NULL DEFAULT 0,
    created INT NOT NULL DEFAULT 0,
    updated INT NOT NULL DEFAULT 0,
    unchanged INT NOT NULL DEFAULT 0,
    error_message LONGTEXT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_user_sync_runs_status (status),
    KEY ix_user_sync_runs_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS expiry_notifications (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT UNSIGNED NOT NULL,
    appointment_id BIGINT UNSIGNED NULL,
    notice_type VARCHAR(20) NOT NULL,
    due_at DATETIME(3) NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',
    sent_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_expiry_notifications_document_id (document_id),
    KEY ix_expiry_notifications_appointment_id (appointment_id),
    KEY ix_expiry_notifications_due_at (due_at),
    UNIQUE KEY ux_expiry_notifications_notice (document_id, appointment_id, notice_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
