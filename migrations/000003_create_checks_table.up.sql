-- 000003_create_checks_table.up.sql

CREATE TABLE IF NOT EXISTS check_logs_raw (
    id BIGSERIAL PRIMARY KEY,
    site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    status_code INT NOT NULL,
    latency_ms INT NOT NULL,
    error_message TEXT DEFAULT '',
    checked_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS check_daily_stats (
    id BIGSERIAL PRIMARY KEY,
    site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    date DATE NOT NULL,                  
    total_checks INT NOT NULL DEFAULT 0,     
    failed_checks INT NOT NULL DEFAULT 0,      
    avg_latency_ms INT NOT NULL DEFAULT 0,   
    max_latency_ms INT NOT NULL DEFAULT 0,    
    uptime_percentage NUMERIC(5,2) NOT NULL,   
  
    CONSTRAINT unique_site_daily_date UNIQUE (site_id, date)
);

CREATE INDEX IF NOT EXISTS idx_logs_raw_site_checked ON check_logs_raw(site_id, checked_at);
CREATE INDEX IF NOT EXISTS idx_daily_stats_site_date ON check_daily_stats(site_id, date DESC);