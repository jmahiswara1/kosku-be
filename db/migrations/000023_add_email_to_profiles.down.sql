DROP INDEX IF EXISTS idx_profiles_email;
ALTER TABLE profiles DROP COLUMN IF EXISTS email;
