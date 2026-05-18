-- Create profiles table (extends Supabase auth.users)
CREATE TABLE profiles (
  id          UUID PRIMARY KEY REFERENCES auth.users(id),
  full_name   TEXT NOT NULL,
  avatar_url  TEXT,
  phone       TEXT,
  role        TEXT NOT NULL CHECK (role IN ('owner', 'tenant', 'staff')),
  created_at  TIMESTAMPTZ DEFAULT NOW(),
  updated_at  TIMESTAMPTZ DEFAULT NOW()
);
