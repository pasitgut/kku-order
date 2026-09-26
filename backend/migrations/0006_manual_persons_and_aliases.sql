ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'SYNCED';
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS created_by VARCHAR(255) NULL;
ALTER TABLE directory_users ADD KEY ix_directory_users_source (source);

CREATE TABLE IF NOT EXISTS person_name_aliases (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    normalized_name VARCHAR(255) NOT NULL,
    directory_user_id BIGINT UNSIGNED NOT NULL,
    source_text VARCHAR(500) NULL,
    created_by VARCHAR(255) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY ux_person_name_aliases_name (normalized_name),
    KEY ix_person_name_aliases_user (directory_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
