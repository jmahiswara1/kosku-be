-- Create properties table
CREATE TABLE properties (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id     UUID NOT NULL REFERENCES profiles(id),
  name         TEXT NOT NULL,
  address      TEXT NOT NULL,
  city         TEXT,
  logo_url     TEXT,
  phone        TEXT,
  bank_name    TEXT,
  bank_account TEXT,
  created_at   TIMESTAMPTZ DEFAULT NOW(),
  updated_at   TIMESTAMPTZ DEFAULT NOW()
);
