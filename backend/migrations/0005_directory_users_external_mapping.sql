ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS user_id VARCHAR(255) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS prefix VARCHAR(100) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS gender VARCHAR(30) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS tel_format VARCHAR(100) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS position_title VARCHAR(255) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS position_en VARCHAR(255) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS prefix_position_en VARCHAR(100) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS manage_position VARCHAR(255) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS name_en VARCHAR(500) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS suffix_en VARCHAR(100) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS scopus_id VARCHAR(100) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS scholar_author_id VARCHAR(255) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS lab_name VARCHAR(500) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS room VARCHAR(100) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS cp_web_id VARCHAR(500) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS role_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS role_name VARCHAR(100) NULL;
ALTER TABLE directory_users ADD COLUMN IF NOT EXISTS source_updated_at VARCHAR(50) NULL;
ALTER TABLE directory_users ADD KEY ix_directory_users_user_id (user_id);

ALTER TABLE directory_users MODIFY COLUMN is_active VARCHAR(20) NOT NULL DEFAULT 'A';
UPDATE directory_users
SET is_active = CASE
    WHEN LOWER(TRIM(is_active)) IN ('1', 'a', 'active', 'true', 'yes', 'y') THEN 'A'
    ELSE 'I'
END;
