-- Create announcements table
CREATE TABLE announcements (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id    UUID NOT NULL REFERENCES profiles(id),
  property_id UUID REFERENCES properties(id),
  title       TEXT NOT NULL,
  body        TEXT NOT NULL,
  send_email  BOOLEAN DEFAULT FALSE,
  created_at  TIMESTAMPTZ DEFAULT NOW()
);
