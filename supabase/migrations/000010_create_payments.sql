-- Create payments table
CREATE TABLE payments (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bill_id          UUID NOT NULL REFERENCES bills(id),
  tenant_id        UUID NOT NULL REFERENCES tenants(id),
  amount           NUMERIC(12,2) NOT NULL,
  proof_url        TEXT,
  status           TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'confirmed', 'rejected')),
  rejection_reason TEXT,
  confirmed_by     UUID REFERENCES profiles(id),
  confirmed_at     TIMESTAMPTZ,
  created_at       TIMESTAMPTZ DEFAULT NOW()
);
