-- 000004_create_refresh_tokens_table.up.sql
-- Хранилище активных refresh-сессий.
-- Сам JWT-токен НЕ храним: в таблице лежит только его jti (RegisteredClaims.ID),
-- привязанный к пользователю. Это позволяет:
--   * отзывать отдельные сессии (logout, ротация);
--   * инвалидировать все сессии пользователя (смена пароля/роли, удаление).
-- Значения jti неугадываемы (UUIDv4), поэтому утечка таблицы не даёт возможности
-- подделать подпись токена (нужен ещё refresh-секрет).

CREATE TABLE IF NOT EXISTS refresh_tokens (
    jti        UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens (expires_at);