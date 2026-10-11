-- +goose Up
ALTER TABLE users ADD COLUMN facebook_id INTEGER;
CREATE UNIQUE INDEX idx_users_facebook_id ON users(facebook_id) WHERE facebook_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_users_facebook_id;
ALTER TABLE users DROP COLUMN facebook_id;

