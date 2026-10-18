CREATE TABLE IF NOT EXISTS position_holders (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    position_key VARCHAR(255) NOT NULL,
    position_title VARCHAR(255) NOT NULL,
    directory_person_id BIGINT NOT NULL,
    starts_on DATE NULL,
    source VARCHAR(20) NOT NULL,
    created_by VARCHAR(255),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    KEY ix_position_holders_key (position_key),
    KEY ix_position_holders_person (directory_person_id),
    CONSTRAINT fk_position_holders_person FOREIGN KEY (directory_person_id) REFERENCES directory_people (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
