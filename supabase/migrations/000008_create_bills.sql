-- Create bills table
CREATE TABLE bills (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id      UUID NOT NULL REFERENCES tenants(id),
  property_id    UUID NOT NULL REFERENCES properties(id),
  room_id        UUID NOT NULL REFERENCES rooms(id),
  period_month   INT NOT NULL,
  period_year    INT NOT NULL,
  base_amount    NUMERIC(12,2) NOT NULL,
  utility_amount NUMERIC(12,2) DEFAULT 0,
  penalty_amount NUMERIC(12,2) DEFAULT 0,
  total_amount   NUMERIC(12,2) GENERATED ALWAYS AS
                   (base_amount + utility_amount + penalty_amount) STORED,
  due_date       DATE NOT NULL,
  status         TEXT NOT NULL DEFAULT 'unpaid'
                   CHECK (status IN ('unpaid', 'pending', 'paid', 'overdue')),
  created_at     TIMESTAMPTZ DEFAULT NOW(),
  updated_at     TIMESTAMPTZ DEFAULT NOW()
);
