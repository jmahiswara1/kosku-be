-- Create utility_charges table
CREATE TABLE utility_charges (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bill_id    UUID NOT NULL REFERENCES bills(id) ON DELETE CASCADE,
  type       TEXT NOT NULL,
  amount     NUMERIC(12,2) NOT NULL,
  note       TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
