-- +goose Up

ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE queues ADD COLUMN priority_default INT NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE queues DROP COLUMN IF EXISTS priority_default;
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
