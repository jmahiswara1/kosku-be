-- Create notifications table
CREATE TABLE notifications (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES profiles(id),
  type       TEXT NOT NULL,
  title      TEXT NOT NULL,
  body       TEXT,
  entity_id  UUID,
  is_read    BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
