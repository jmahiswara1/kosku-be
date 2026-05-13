-- Create room_types table
CREATE TABLE room_types (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  property_id   UUID NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  monthly_price NUMERIC(12,2) NOT NULL,
  created_at    TIMESTAMPTZ DEFAULT NOW()
);
