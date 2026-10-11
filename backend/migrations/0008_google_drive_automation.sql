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
    imported_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
