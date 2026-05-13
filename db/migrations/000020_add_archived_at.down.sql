-- Revert soft-archive columns
ALTER TABLE tenants    DROP COLUMN IF EXISTS archived_at;
ALTER TABLE rooms      DROP COLUMN IF EXISTS archived_at;
ALTER TABLE properties DROP COLUMN IF EXISTS archived_at;
