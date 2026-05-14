-- Add billing configuration fields to properties table
ALTER TABLE properties
  ADD COLUMN IF NOT EXISTS due_date_day      INT,
  ADD COLUMN IF NOT EXISTS grace_period_days INT,
  ADD COLUMN IF NOT EXISTS penalty_type      TEXT CHECK (penalty_type IN ('flat', 'percentage')),
  ADD COLUMN IF NOT EXISTS penalty_amount    NUMERIC(12,2);
