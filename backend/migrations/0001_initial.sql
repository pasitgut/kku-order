DROP TABLE IF EXISTS expiry_notifications;
DROP TABLE IF EXISTS document_revisions;
DROP TABLE IF EXISTS document_references;
DROP TABLE IF EXISTS extracted_fields;
DROP TABLE IF EXISTS ocr_lines;
DROP TABLE IF EXISTS ocr_pages;
DROP TABLE IF EXISTS person_name_aliases;
DROP TABLE IF EXISTS directory_person_profiles;
DROP TABLE IF EXISTS application_audit_logs;
DROP TABLE IF EXISTS document_workflows;
DROP TABLE IF EXISTS appointments;
DROP TABLE IF EXISTS committee_members;
DROP TABLE IF EXISTS committees;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS committee_roles;
DROP TABLE IF EXISTS order_types;
DROP TABLE IF EXISTS directory_people;
DROP TABLE IF EXISTS directory_users;
DROP TABLE IF EXISTS user_sync_runs;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS audit_logs;

CREATE TABLE documents (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    file_name VARCHAR(500) NOT NULL,
    storage_path VARCHAR(1000) NOT NULL,
    file_type VARCHAR(20) NOT NULL,
    file_hash CHAR(64) NOT NULL UNIQUE,
    confirmed_at TIMESTAMP NULL,
    confirmed_by BIGINT NULL,
    created_at TIMESTAMP NULL,
    updated_at TIMESTAMP NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE directory_people (
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

CREATE TABLE committees (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    order_id BIGINT NOT NULL,
    name VARCHAR(500),
    description TEXT,
    created_at TIMESTAMP NULL,
    updated_at TIMESTAMP NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE committee_members (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    committee_id BIGINT NOT NULL,
    people_id BIGINT,
    committee_role_id BIGINT NOT NULL,
    display_order INT,
    created_at TIMESTAMP NULL,
    updated_at TIMESTAMP NULL,
    UNIQUE KEY ux_committee_members_committee_people (committee_id, people_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE committee_roles (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    code VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL UNIQUE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE orders (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    document_id BIGINT NOT NULL UNIQUE,
    order_type_id BIGINT NOT NULL,
    order_number VARCHAR(50) NOT NULL,
    order_year INT NOT NULL,
    title VARCHAR(255),
    issued_date DATETIME NULL,
    effective_date DATETIME NULL,
    expiry_date DATETIME NULL,
    created_at TIMESTAMP NULL,
    updated_at TIMESTAMP NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE order_types (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE users (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE audit_logs (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE documents
    ADD CONSTRAINT fk_documents_confirmed_by
        FOREIGN KEY (confirmed_by) REFERENCES users (id),
    ADD CONSTRAINT fk_documents_id_orders_document_id
        FOREIGN KEY (id) REFERENCES orders (document_id);

ALTER TABLE orders
    ADD CONSTRAINT fk_orders_order_type
        FOREIGN KEY (order_type_id) REFERENCES order_types (id);

ALTER TABLE committees
    ADD CONSTRAINT fk_committees_order
        FOREIGN KEY (order_id) REFERENCES orders (id);

ALTER TABLE committee_members
    ADD CONSTRAINT fk_committee_members_committee
        FOREIGN KEY (committee_id) REFERENCES committees (id),
    ADD CONSTRAINT fk_committee_members_person
        FOREIGN KEY (people_id) REFERENCES directory_people (id),
    ADD CONSTRAINT fk_committee_members_role
        FOREIGN KEY (committee_role_id) REFERENCES committee_roles (id);
