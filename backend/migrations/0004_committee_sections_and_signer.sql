ALTER TABLE documents ADD COLUMN IF NOT EXISTS signer_position VARCHAR(255) NULL;
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS department VARCHAR(500) NULL;
ALTER TABLE appointments ADD KEY ix_appointments_department (department(191));
