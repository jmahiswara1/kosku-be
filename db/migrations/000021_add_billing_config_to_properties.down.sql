-- Remove billing configuration fields from properties table
ALTER TABLE properties
  DROP COLUMN IF EXISTS due_date_day,
  DROP COLUMN IF EXISTS grace_period_days,
  DROP COLUMN IF EXISTS penalty_type,
  DROP COLUMN IF EXISTS penalty_amount;
