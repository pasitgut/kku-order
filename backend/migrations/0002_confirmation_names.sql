CREATE TABLE IF NOT EXISTS directory_previous_names (
    directory_person_id BIGINT NOT NULL,
    normalized_name VARCHAR(511) COLLATE utf8mb4_bin NOT NULL,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    source VARCHAR(20) NOT NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (directory_person_id, normalized_name),
    KEY ix_directory_previous_names_name (normalized_name),
    CONSTRAINT fk_previous_names_person FOREIGN KEY (directory_person_id) REFERENCES directory_people (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS document_confirmations (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT NOT NULL,
    confirmed_by VARCHAR(255) NOT NULL,
    confirmed_at DATETIME(3) NOT NULL,
    names JSON NOT NULL,
    KEY ix_document_confirmations_document (document_id, id),
    CONSTRAINT fk_document_confirmations_document FOREIGN KEY (document_id) REFERENCES documents (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
