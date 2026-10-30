ALTER TABLE directory_person_profiles ADD COLUMN IF NOT EXISTS local_overrides JSON NULL;
ALTER TABLE position_holders ADD COLUMN IF NOT EXISTS ends_on DATE NULL;

CREATE TABLE IF NOT EXISTS directory_person_revisions (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    directory_person_id BIGINT NOT NULL,
    changes JSON NOT NULL,
    changed_by VARCHAR(255) NOT NULL,
    changed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY ix_directory_person_revisions_person_changed (directory_person_id, changed_at),
    CONSTRAINT fk_directory_person_revisions_person FOREIGN KEY (directory_person_id) REFERENCES directory_people (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
