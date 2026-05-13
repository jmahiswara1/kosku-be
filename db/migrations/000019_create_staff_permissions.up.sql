-- Create staff_permissions table
CREATE TABLE staff_permissions (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  staff_id   UUID NOT NULL REFERENCES profiles(id),
  owner_id   UUID NOT NULL REFERENCES profiles(id),
  modules    JSONB NOT NULL DEFAULT '[]',
  created_at TIMESTAMPTZ DEFAULT NOW()
);
