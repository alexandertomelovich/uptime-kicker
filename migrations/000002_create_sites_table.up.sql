-- 000002_create_sites_table.up.sql

CREATE TABLE IF NOT EXISTS sites (
    id UUID PRIMARY KEY DEFAULT get_random_uuid(),
    url VARCHAR(500) NOT NULL,
    name VARCHAR(255) NOT NULL,
    check_interval_seconds INT NOT NULL DEFAULT 60,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(20) DEFAULT 'pending',
    last_status_code INT,
    last_checked_at TIMESTAMPTZ,
    response_time_ms INT,
    is_active BOOLEAN DEFAULT TRUE,
    verified_at TIMESTAMPTZ,
    verification_token VARCHAR(64),
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT valid_status CHECK (status IN ('up', 'down', 'pending', 'maintenance'))
);

CREATE INDEX idx_sites_user_id ON sites(user_id);
CREATE INDEX idx_sites_status ON sites(status);
CREATE INDEX idx_sites_last_checked ON sites(last_checked_at);
CREATE INDEX idx_sites_active_status ON sites(is_active, status);