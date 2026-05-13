-- Create room_photos table
CREATE TABLE room_photos (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  room_id    UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  url        TEXT NOT NULL,
  order_idx  INT DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
