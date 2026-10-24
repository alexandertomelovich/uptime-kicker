ALTER TABLE sites
    ADD COLUMN verified_at TIMESTAMPTZ,
    ADD COLUMN verification_token VARCHAR(64);
