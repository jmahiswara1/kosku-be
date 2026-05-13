-- Create contracts table
CREATE TABLE contracts (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id        UUID NOT NULL REFERENCES tenants(id),
  room_id          UUID NOT NULL REFERENCES rooms(id),
  property_id      UUID NOT NULL REFERENCES properties(id),
  start_date       DATE NOT NULL,
  end_date         DATE NOT NULL,
  monthly_price    NUMERIC(12,2) NOT NULL,
  deposit_amount   NUMERIC(12,2) DEFAULT 0,
  deposit_refunded NUMERIC(12,2) DEFAULT 0,
  status           TEXT NOT NULL DEFAULT 'active'
                     CHECK (status IN ('active', 'expired', 'terminated')),
  file_url         TEXT,
  created_at       TIMESTAMPTZ DEFAULT NOW(),
  updated_at       TIMESTAMPTZ DEFAULT NOW()
);
