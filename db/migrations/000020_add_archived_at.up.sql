-- Add soft-archive support to properties, rooms, and tenants
ALTER TABLE properties ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
ALTER TABLE rooms      ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
ALTER TABLE tenants    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
