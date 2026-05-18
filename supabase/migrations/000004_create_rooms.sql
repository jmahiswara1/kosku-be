-- Create rooms table
CREATE TABLE rooms (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  property_id  UUID NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
  room_type_id UUID REFERENCES room_types(id),
  number       TEXT NOT NULL,
  floor        INT,
  status       TEXT NOT NULL DEFAULT 'vacant'
                 CHECK (status IN ('vacant', 'occupied', 'pending', 'maintenance')),
  grid_x       INT,
  grid_y       INT,
  facilities   JSONB DEFAULT '[]',
  created_at   TIMESTAMPTZ DEFAULT NOW(),
  updated_at   TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE (property_id, number)
);
