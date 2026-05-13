-- Create tenants table (extended profile for tenant role)
CREATE TABLE tenants (
  id               UUID PRIMARY KEY REFERENCES profiles(id),
  property_id      UUID REFERENCES properties(id),
  room_id          UUID REFERENCES rooms(id),
  ktp_number       TEXT,
  ktp_scan_url     TEXT,
  occupation       TEXT,
  emergency_name   TEXT,
  emergency_phone  TEXT,
  is_blacklisted   BOOLEAN DEFAULT FALSE,
  blacklist_reason TEXT,
  created_at       TIMESTAMPTZ DEFAULT NOW(),
  updated_at       TIMESTAMPTZ DEFAULT NOW()
);
