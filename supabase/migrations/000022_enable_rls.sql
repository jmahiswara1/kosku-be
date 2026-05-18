-- Enable Row Level Security on all tables
ALTER TABLE profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE properties ENABLE ROW LEVEL SECURITY;
ALTER TABLE room_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE rooms ENABLE ROW LEVEL SECURITY;
ALTER TABLE room_photos ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE contracts ENABLE ROW LEVEL SECURITY;
ALTER TABLE bills ENABLE ROW LEVEL SECURITY;
ALTER TABLE utility_charges ENABLE ROW LEVEL SECURITY;
ALTER TABLE payments ENABLE ROW LEVEL SECURITY;
ALTER TABLE tickets ENABLE ROW LEVEL SECURITY;
ALTER TABLE ticket_attachments ENABLE ROW LEVEL SECURITY;
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE announcements ENABLE ROW LEVEL SECURITY;
ALTER TABLE contract_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE staff_permissions ENABLE ROW LEVEL SECURITY;

-- ── Profiles ──────────────────────────────────────────────────────────────────
-- Users can read and update their own profile only
CREATE POLICY "profiles_select_own" ON profiles
  FOR SELECT USING (auth.uid() = id);

CREATE POLICY "profiles_update_own" ON profiles
  FOR UPDATE USING (auth.uid() = id);

-- Service role can do everything (used by backend)
CREATE POLICY "profiles_service_role" ON profiles
  FOR ALL USING (auth.role() = 'service_role');

-- ── Properties ────────────────────────────────────────────────────────────────
-- Owners can only access their own properties
CREATE POLICY "properties_owner_all" ON properties
  FOR ALL USING (owner_id = auth.uid() OR auth.role() = 'service_role');

-- ── Room Types ────────────────────────────────────────────────────────────────
CREATE POLICY "room_types_owner_all" ON room_types
  FOR ALL USING (
    property_id IN (SELECT id FROM properties WHERE owner_id = auth.uid())
    OR auth.role() = 'service_role'
  );

-- ── Rooms ─────────────────────────────────────────────────────────────────────
CREATE POLICY "rooms_owner_all" ON rooms
  FOR ALL USING (
    property_id IN (SELECT id FROM properties WHERE owner_id = auth.uid())
    OR auth.role() = 'service_role'
  );

-- Tenants can read rooms in their assigned property
CREATE POLICY "rooms_tenant_select" ON rooms
  FOR SELECT USING (
    property_id IN (SELECT property_id FROM tenants WHERE id = auth.uid())
    OR auth.role() = 'service_role'
  );

-- ── Room Photos ───────────────────────────────────────────────────────────────
CREATE POLICY "room_photos_owner_all" ON room_photos
  FOR ALL USING (
    room_id IN (
      SELECT r.id FROM rooms r
      JOIN properties p ON r.property_id = p.id
      WHERE p.owner_id = auth.uid()
    )
    OR auth.role() = 'service_role'
  );

-- ── Tenants ───────────────────────────────────────────────────────────────────
-- Owners can manage tenants in their properties
CREATE POLICY "tenants_owner_all" ON tenants
  FOR ALL USING (
    property_id IN (SELECT id FROM properties WHERE owner_id = auth.uid())
    OR auth.role() = 'service_role'
  );

-- Tenants can read and update their own record
CREATE POLICY "tenants_self_select" ON tenants
  FOR SELECT USING (id = auth.uid() OR auth.role() = 'service_role');

CREATE POLICY "tenants_self_update" ON tenants
  FOR UPDATE USING (id = auth.uid() OR auth.role() = 'service_role');

-- ── Contracts ─────────────────────────────────────────────────────────────────
CREATE POLICY "contracts_owner_all" ON contracts
  FOR ALL USING (
    property_id IN (SELECT id FROM properties WHERE owner_id = auth.uid())
    OR auth.role() = 'service_role'
  );

CREATE POLICY "contracts_tenant_select" ON contracts
  FOR SELECT USING (tenant_id = auth.uid() OR auth.role() = 'service_role');

-- ── Bills ─────────────────────────────────────────────────────────────────────
CREATE POLICY "bills_owner_all" ON bills
  FOR ALL USING (
    property_id IN (SELECT id FROM properties WHERE owner_id = auth.uid())
    OR auth.role() = 'service_role'
  );

CREATE POLICY "bills_tenant_select" ON bills
  FOR SELECT USING (tenant_id = auth.uid() OR auth.role() = 'service_role');

-- ── Utility Charges ───────────────────────────────────────────────────────────
CREATE POLICY "utility_charges_service_role" ON utility_charges
  FOR ALL USING (auth.role() = 'service_role');

-- ── Payments ──────────────────────────────────────────────────────────────────
CREATE POLICY "payments_owner_all" ON payments
  FOR ALL USING (
    bill_id IN (
      SELECT b.id FROM bills b
      JOIN properties p ON b.property_id = p.id
      WHERE p.owner_id = auth.uid()
    )
    OR auth.role() = 'service_role'
  );

CREATE POLICY "payments_tenant_all" ON payments
  FOR ALL USING (tenant_id = auth.uid() OR auth.role() = 'service_role');

-- ── Tickets ───────────────────────────────────────────────────────────────────
CREATE POLICY "tickets_owner_all" ON tickets
  FOR ALL USING (
    property_id IN (SELECT id FROM properties WHERE owner_id = auth.uid())
    OR auth.role() = 'service_role'
  );

CREATE POLICY "tickets_tenant_all" ON tickets
  FOR ALL USING (tenant_id = auth.uid() OR auth.role() = 'service_role');

-- ── Ticket Attachments ────────────────────────────────────────────────────────
CREATE POLICY "ticket_attachments_service_role" ON ticket_attachments
  FOR ALL USING (auth.role() = 'service_role');

-- ── Notifications ─────────────────────────────────────────────────────────────
CREATE POLICY "notifications_own" ON notifications
  FOR ALL USING (user_id = auth.uid() OR auth.role() = 'service_role');

-- ── Audit Logs ────────────────────────────────────────────────────────────────
-- Read-only for owners, full access for service role
CREATE POLICY "audit_logs_owner_select" ON audit_logs
  FOR SELECT USING (
    actor_id = auth.uid()
    OR auth.role() = 'service_role'
  );

CREATE POLICY "audit_logs_service_insert" ON audit_logs
  FOR INSERT WITH CHECK (auth.role() = 'service_role');

-- ── Invitations ───────────────────────────────────────────────────────────────
CREATE POLICY "invitations_owner_all" ON invitations
  FOR ALL USING (owner_id = auth.uid() OR auth.role() = 'service_role');

-- ── Messages ──────────────────────────────────────────────────────────────────
CREATE POLICY "messages_participants" ON messages
  FOR ALL USING (
    sender_id = auth.uid()
    OR receiver_id = auth.uid()
    OR auth.role() = 'service_role'
  );

-- ── Announcements ─────────────────────────────────────────────────────────────
CREATE POLICY "announcements_owner_all" ON announcements
  FOR ALL USING (owner_id = auth.uid() OR auth.role() = 'service_role');

-- ── Contract Templates ────────────────────────────────────────────────────────
CREATE POLICY "contract_templates_owner_all" ON contract_templates
  FOR ALL USING (owner_id = auth.uid() OR auth.role() = 'service_role');

-- ── Staff Permissions ─────────────────────────────────────────────────────────
CREATE POLICY "staff_permissions_owner_all" ON staff_permissions
  FOR ALL USING (owner_id = auth.uid() OR auth.role() = 'service_role');

CREATE POLICY "staff_permissions_staff_select" ON staff_permissions
  FOR SELECT USING (staff_id = auth.uid() OR auth.role() = 'service_role');
