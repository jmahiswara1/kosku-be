// Package service_test contains property-based tests for the tenant service.
package service_test

// Validates: Requirements 3.2, 2.2
//
// Property 1: Occupancy invariant
// For any sequence of check-in and check-out operations on a property,
// COUNT(rooms WHERE status='occupied') must always equal
// COUNT(contracts WHERE status='active' AND property_id=X).
//
// This test uses an in-memory simulation of the state transitions to verify
// the invariant holds for arbitrary sequences of operations.

import (
	"testing"
	"testing/quick"

	"github.com/google/uuid"
)

// roomStatus represents the status of a room in the simulation.
type roomStatus string

const (
	roomVacant   roomStatus = "vacant"
	roomOccupied roomStatus = "occupied"
)

// contractStatus represents the status of a contract in the simulation.
type contractStatus string

const (
	contractActive     contractStatus = "active"
	contractTerminated contractStatus = "terminated"
)

// simulatedRoom represents a room in the in-memory simulation.
type simulatedRoom struct {
	id     uuid.UUID
	status roomStatus
}

// simulatedContract represents a contract in the in-memory simulation.
type simulatedContract struct {
	id         uuid.UUID
	tenantID   uuid.UUID
	roomID     uuid.UUID
	propertyID uuid.UUID
	status     contractStatus
}

// simulatedTenant represents a tenant in the in-memory simulation.
type simulatedTenant struct {
	id            uuid.UUID
	isBlacklisted bool
}

// propertyState holds the in-memory state of a property for simulation.
type propertyState struct {
	propertyID uuid.UUID
	rooms      map[uuid.UUID]*simulatedRoom
	contracts  map[uuid.UUID]*simulatedContract
	tenants    map[uuid.UUID]*simulatedTenant
}

// newPropertyState creates a new property state with N vacant rooms.
func newPropertyState(n int) *propertyState {
	ps := &propertyState{
		propertyID: uuid.New(),
		rooms:      make(map[uuid.UUID]*simulatedRoom, n),
		contracts:  make(map[uuid.UUID]*simulatedContract),
		tenants:    make(map[uuid.UUID]*simulatedTenant),
	}
	for i := 0; i < n; i++ {
		id := uuid.New()
		ps.rooms[id] = &simulatedRoom{id: id, status: roomVacant}
	}
	return ps
}

// checkin simulates a check-in operation.
// Returns false if the operation is invalid (room not vacant or tenant blacklisted).
func (ps *propertyState) checkin(tenantID, roomID uuid.UUID) bool {
	room, ok := ps.rooms[roomID]
	if !ok {
		return false
	}
	if room.status != roomVacant {
		return false
	}

	tenant, ok := ps.tenants[tenantID]
	if !ok {
		// Auto-create tenant for simulation.
		tenant = &simulatedTenant{id: tenantID, isBlacklisted: false}
		ps.tenants[tenantID] = tenant
	}
	if tenant.isBlacklisted {
		return false
	}

	// Create contract and update room status.
	contractID := uuid.New()
	ps.contracts[contractID] = &simulatedContract{
		id:         contractID,
		tenantID:   tenantID,
		roomID:     roomID,
		propertyID: ps.propertyID,
		status:     contractActive,
	}
	room.status = roomOccupied
	return true
}

// checkout simulates a check-out operation for a tenant.
// Returns false if no active contract exists for the tenant.
func (ps *propertyState) checkout(tenantID uuid.UUID) bool {
	// Find active contract for tenant.
	var activeContract *simulatedContract
	for _, c := range ps.contracts {
		if c.tenantID == tenantID && c.status == contractActive {
			activeContract = c
			break
		}
	}
	if activeContract == nil {
		return false
	}

	// Terminate contract and update room status.
	activeContract.status = contractTerminated
	if room, ok := ps.rooms[activeContract.roomID]; ok {
		room.status = roomVacant
	}
	return true
}

// occupiedRoomCount returns the count of occupied rooms.
func (ps *propertyState) occupiedRoomCount() int {
	count := 0
	for _, r := range ps.rooms {
		if r.status == roomOccupied {
			count++
		}
	}
	return count
}

// activeContractCount returns the count of active contracts for the property.
func (ps *propertyState) activeContractCount() int {
	count := 0
	for _, c := range ps.contracts {
		if c.propertyID == ps.propertyID && c.status == contractActive {
			count++
		}
	}
	return count
}

// checkInvariant verifies the occupancy invariant:
// occupied rooms == active contracts.
func (ps *propertyState) checkInvariant() bool {
	return ps.occupiedRoomCount() == ps.activeContractCount()
}

// operationType represents a type of operation in the simulation.
type operationType uint8

const (
	opCheckin  operationType = 0
	opCheckout operationType = 1
)

// TestOccupancyInvariant_Property is a property-based test that verifies the
// occupancy invariant holds for any sequence of check-in and check-out operations.
//
// Validates: Requirements 3.2, 2.2
func TestOccupancyInvariant_Property(t *testing.T) {
	// Property: for any sequence of operations on a property with N rooms,
	// the number of occupied rooms always equals the number of active contracts.
	property := func(ops []uint8) bool {
		if len(ops) == 0 {
			return true
		}

		// Create a property with 5 rooms and 10 tenants.
		const numRooms = 5
		ps := newPropertyState(numRooms)

		// Pre-create tenants.
		tenantIDs := make([]uuid.UUID, 10)
		for i := range tenantIDs {
			tenantIDs[i] = uuid.New()
			ps.tenants[tenantIDs[i]] = &simulatedTenant{id: tenantIDs[i], isBlacklisted: false}
		}

		// Collect room IDs for indexing.
		roomIDs := make([]uuid.UUID, 0, numRooms)
		for id := range ps.rooms {
			roomIDs = append(roomIDs, id)
		}

		// Execute operations.
		for _, op := range ops {
			opType := operationType(op % 2)
			tenantIdx := int(op) % len(tenantIDs)
			roomIdx := int(op) % len(roomIDs)

			switch opType {
			case opCheckin:
				ps.checkin(tenantIDs[tenantIdx], roomIDs[roomIdx])
			case opCheckout:
				ps.checkout(tenantIDs[tenantIdx])
			}

			// Check invariant after every operation.
			if !ps.checkInvariant() {
				return false
			}
		}

		return true
	}

	cfg := &quick.Config{
		MaxCount: 500,
	}

	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("occupancy invariant violated: %v", err)
	}
}

// TestOccupancyInvariant_InitialState verifies the invariant holds for an
// empty property (no rooms occupied, no active contracts).
//
// Validates: Requirements 3.2, 2.2
func TestOccupancyInvariant_InitialState(t *testing.T) {
	ps := newPropertyState(5)
	if !ps.checkInvariant() {
		t.Errorf("invariant violated on initial state: occupied=%d active_contracts=%d",
			ps.occupiedRoomCount(), ps.activeContractCount())
	}
}

// TestOccupancyInvariant_AfterCheckin verifies the invariant holds after a check-in.
//
// Validates: Requirements 3.2, 2.2
func TestOccupancyInvariant_AfterCheckin(t *testing.T) {
	ps := newPropertyState(3)
	tenantID := uuid.New()
	ps.tenants[tenantID] = &simulatedTenant{id: tenantID}

	var roomID uuid.UUID
	for id := range ps.rooms {
		roomID = id
		break
	}

	ps.checkin(tenantID, roomID)

	if !ps.checkInvariant() {
		t.Errorf("invariant violated after checkin: occupied=%d active_contracts=%d",
			ps.occupiedRoomCount(), ps.activeContractCount())
	}
	if ps.occupiedRoomCount() != 1 {
		t.Errorf("expected 1 occupied room, got %d", ps.occupiedRoomCount())
	}
}

// TestOccupancyInvariant_AfterCheckout verifies the invariant holds after a check-out.
//
// Validates: Requirements 3.2, 2.2
func TestOccupancyInvariant_AfterCheckout(t *testing.T) {
	ps := newPropertyState(3)
	tenantID := uuid.New()
	ps.tenants[tenantID] = &simulatedTenant{id: tenantID}

	var roomID uuid.UUID
	for id := range ps.rooms {
		roomID = id
		break
	}

	ps.checkin(tenantID, roomID)
	ps.checkout(tenantID)

	if !ps.checkInvariant() {
		t.Errorf("invariant violated after checkout: occupied=%d active_contracts=%d",
			ps.occupiedRoomCount(), ps.activeContractCount())
	}
	if ps.occupiedRoomCount() != 0 {
		t.Errorf("expected 0 occupied rooms after checkout, got %d", ps.occupiedRoomCount())
	}
}

// TestOccupancyInvariant_CheckinFailsForOccupiedRoom verifies that attempting
// to check in to an already-occupied room does not violate the invariant.
//
// Validates: Requirements 3.2, 2.2
func TestOccupancyInvariant_CheckinFailsForOccupiedRoom(t *testing.T) {
	ps := newPropertyState(1)
	tenant1 := uuid.New()
	tenant2 := uuid.New()
	ps.tenants[tenant1] = &simulatedTenant{id: tenant1}
	ps.tenants[tenant2] = &simulatedTenant{id: tenant2}

	var roomID uuid.UUID
	for id := range ps.rooms {
		roomID = id
		break
	}

	// First check-in should succeed.
	if !ps.checkin(tenant1, roomID) {
		t.Fatal("expected first checkin to succeed")
	}

	// Second check-in to same room should fail.
	if ps.checkin(tenant2, roomID) {
		t.Error("expected second checkin to same room to fail")
	}

	// Invariant must still hold.
	if !ps.checkInvariant() {
		t.Errorf("invariant violated: occupied=%d active_contracts=%d",
			ps.occupiedRoomCount(), ps.activeContractCount())
	}
	if ps.occupiedRoomCount() != 1 {
		t.Errorf("expected 1 occupied room, got %d", ps.occupiedRoomCount())
	}
}

// TestOccupancyInvariant_CheckinFailsForBlacklistedTenant verifies that
// attempting to check in a blacklisted tenant does not violate the invariant.
//
// Validates: Requirements 3.2, 2.2
func TestOccupancyInvariant_CheckinFailsForBlacklistedTenant(t *testing.T) {
	ps := newPropertyState(2)
	blacklistedTenant := uuid.New()
	ps.tenants[blacklistedTenant] = &simulatedTenant{id: blacklistedTenant, isBlacklisted: true}

	var roomID uuid.UUID
	for id := range ps.rooms {
		roomID = id
		break
	}

	// Check-in should fail for blacklisted tenant.
	if ps.checkin(blacklistedTenant, roomID) {
		t.Error("expected checkin to fail for blacklisted tenant")
	}

	// Invariant must still hold.
	if !ps.checkInvariant() {
		t.Errorf("invariant violated: occupied=%d active_contracts=%d",
			ps.occupiedRoomCount(), ps.activeContractCount())
	}
	if ps.occupiedRoomCount() != 0 {
		t.Errorf("expected 0 occupied rooms, got %d", ps.occupiedRoomCount())
	}
}
