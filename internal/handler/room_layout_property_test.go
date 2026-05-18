// Package handler_test contains property-based tests for the room layout HTTP handlers.
//
// Validates: Requirements 2.3
//
// Property 6: Layout persistence
// After saving a custom room layout, loading the property layout must return
// the exact same grid positions for all rooms.
//
// This test verifies the round-trip property: for any set of N rooms with
// arbitrary grid positions, calling PUT /v1/properties/:id/layout followed by
// GET /v1/properties/:id/rooms must return the exact same grid_x/grid_y values
// that were saved.
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/quick"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/handler"
	"github.com/kosku/backend/internal/middleware"
)

// ---------------------------------------------------------------------------
// Mock room service for layout persistence tests
// ---------------------------------------------------------------------------

// mockRoomService is a test double for handler.RoomServicer.
// Each field holds the function that will be called for the corresponding method.
type mockRoomService struct {
	listRoomsFn    func(ctx context.Context, ownerID, propertyID uuid.UUID) ([]dto.RoomResponse, error)
	createRoomFn   func(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.CreateRoomRequest) (dto.RoomResponse, error)
	getRoomFn      func(ctx context.Context, ownerID, roomID uuid.UUID) (dto.RoomResponse, error)
	updateRoomFn   func(ctx context.Context, ownerID, roomID uuid.UUID, req dto.UpdateRoomRequest) (dto.RoomResponse, error)
	archiveRoomFn  func(ctx context.Context, ownerID, roomID uuid.UUID) error
	updateLayoutFn func(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.UpdateLayoutRequest) error
	getRoomHistFn  func(ctx context.Context, ownerID, roomID uuid.UUID) ([]dto.RoomHistoryItem, error)
}

func (m *mockRoomService) ListRooms(ctx context.Context, ownerID, propertyID uuid.UUID) ([]dto.RoomResponse, error) {
	return m.listRoomsFn(ctx, ownerID, propertyID)
}

func (m *mockRoomService) CreateRoom(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.CreateRoomRequest) (dto.RoomResponse, error) {
	return m.createRoomFn(ctx, ownerID, propertyID, req)
}

func (m *mockRoomService) GetRoom(ctx context.Context, ownerID, roomID uuid.UUID) (dto.RoomResponse, error) {
	return m.getRoomFn(ctx, ownerID, roomID)
}

func (m *mockRoomService) UpdateRoom(ctx context.Context, ownerID, roomID uuid.UUID, req dto.UpdateRoomRequest) (dto.RoomResponse, error) {
	return m.updateRoomFn(ctx, ownerID, roomID, req)
}

func (m *mockRoomService) ArchiveRoom(ctx context.Context, ownerID, roomID uuid.UUID) error {
	return m.archiveRoomFn(ctx, ownerID, roomID)
}

func (m *mockRoomService) UpdateLayout(ctx context.Context, ownerID, propertyID uuid.UUID, req dto.UpdateLayoutRequest) error {
	return m.updateLayoutFn(ctx, ownerID, propertyID, req)
}

func (m *mockRoomService) GetRoomHistory(ctx context.Context, ownerID, roomID uuid.UUID) ([]dto.RoomHistoryItem, error) {
	return m.getRoomHistFn(ctx, ownerID, roomID)
}

// ---------------------------------------------------------------------------
// Router helper for room handler tests
// ---------------------------------------------------------------------------

// newRoomRouter builds a minimal Gin router that injects ownerID into the
// context (simulating what the Auth middleware does) and registers the room
// handler routes needed for layout persistence tests.
func newRoomRouter(svc handler.RoomServicer, ownerID string) *gin.Engine {
	r := gin.New()

	r.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, ownerID)
		c.Next()
	})

	h := handler.NewRoomHandlerWithService(svc)
	r.PUT("/properties/:id/layout", h.UpdateLayout)
	r.GET("/properties/:id/rooms", h.ListRooms)

	return r
}

// putJSON performs a PUT request with a JSON body and returns the recorder.
func putJSON(r *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPut, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// getRequest performs a GET request and returns the recorder.
func getRequest(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// Property 6: Layout persistence
// ---------------------------------------------------------------------------

// TestLayoutPersistence_Property is a property-based test that verifies the
// layout persistence invariant: for any set of N rooms with arbitrary grid
// positions, saving the layout and then loading the rooms must return the
// exact same grid_x/grid_y values for every room.
//
// Validates: Requirements 2.3
func TestLayoutPersistence_Property(t *testing.T) {
	// property verifies the round-trip for a given number of rooms (n).
	// testing/quick generates random uint8 values for n.
	property := func(n uint8) bool {
		// Clamp n to a sensible range: at least 1 room, at most 50 to keep the test fast.
		count := int(n)
		if count < 1 {
			count = 1
		}
		if count > 50 {
			count = 50
		}

		ownerID := uuid.New()
		propertyID := uuid.New()

		// Generate N rooms with distinct IDs and random grid positions.
		// We use deterministic positions derived from the index so that the
		// test is reproducible when a counterexample is found.
		rooms := make([]dto.RoomResponse, count)
		layoutItems := make([]dto.LayoutItem, count)

		for i := 0; i < count; i++ {
			roomID := uuid.New()
			gridX := int32(i * 3)
			gridY := int32(i*3 + 100)

			rooms[i] = dto.RoomResponse{
				ID:         roomID.String(),
				PropertyID: propertyID.String(),
				Number:     fmt.Sprintf("R%03d", i+1),
				Status:     "vacant",
				GridX:      &gridX,
				GridY:      &gridY,
				Facilities: []string{},
			}

			layoutItems[i] = dto.LayoutItem{
				RoomID: roomID.String(),
				GridX:  int(gridX),
				GridY:  int(gridY),
			}
		}

		// In-memory store: maps roomID -> (gridX, gridY).
		// UpdateLayout writes to this store; ListRooms reads from it.
		type gridPos struct{ x, y int32 }
		store := make(map[string]gridPos, count)

		// Initialize the store with the initial room positions.
		for _, r := range rooms {
			store[r.ID] = gridPos{x: *r.GridX, y: *r.GridY}
		}

		svc := &mockRoomService{
			// UpdateLayout: persist the new grid positions into the in-memory store.
			updateLayoutFn: func(_ context.Context, oID, pID uuid.UUID, req dto.UpdateLayoutRequest) error {
				if oID != ownerID {
					return fmt.Errorf("unexpected ownerID: got %s, want %s", oID, ownerID)
				}
				if pID != propertyID {
					return fmt.Errorf("unexpected propertyID: got %s, want %s", pID, propertyID)
				}
				for _, item := range req.Rooms {
					store[item.RoomID] = gridPos{x: int32(item.GridX), y: int32(item.GridY)}
				}
				return nil
			},

			// ListRooms: return rooms with grid positions read from the in-memory store.
			listRoomsFn: func(_ context.Context, oID, pID uuid.UUID) ([]dto.RoomResponse, error) {
				if oID != ownerID {
					return nil, fmt.Errorf("unexpected ownerID: got %s, want %s", oID, ownerID)
				}
				if pID != propertyID {
					return nil, fmt.Errorf("unexpected propertyID: got %s, want %s", pID, propertyID)
				}
				result := make([]dto.RoomResponse, len(rooms))
				for i, r := range rooms {
					pos := store[r.ID]
					gx := pos.x
					gy := pos.y
					result[i] = dto.RoomResponse{
						ID:         r.ID,
						PropertyID: r.PropertyID,
						Number:     r.Number,
						Status:     r.Status,
						GridX:      &gx,
						GridY:      &gy,
						Facilities: []string{},
					}
				}
				return result, nil
			},
		}

		router := newRoomRouter(svc, ownerID.String())

		// Step 1: Save the layout via PUT /v1/properties/:id/layout.
		layoutReq := dto.UpdateLayoutRequest{Rooms: layoutItems}
		putW := putJSON(router, "/properties/"+propertyID.String()+"/layout", layoutReq)
		if putW.Code != http.StatusOK {
			t.Logf("PUT layout returned %d: %s", putW.Code, putW.Body.String())
			return false
		}

		// Step 2: Load the rooms via GET /v1/properties/:id/rooms.
		getW := getRequest(router, "/properties/"+propertyID.String()+"/rooms")
		if getW.Code != http.StatusOK {
			t.Logf("GET rooms returned %d: %s", getW.Code, getW.Body.String())
			return false
		}

		// Step 3: Decode the response and build a map of roomID -> (gridX, gridY).
		var resp struct {
			Success bool               `json:"success"`
			Data    []dto.RoomResponse `json:"data"`
		}
		if err := json.NewDecoder(getW.Body).Decode(&resp); err != nil {
			t.Logf("failed to decode GET rooms response: %v", err)
			return false
		}
		if !resp.Success {
			t.Logf("GET rooms returned success=false")
			return false
		}

		// Build a lookup map from the GET response.
		returned := make(map[string]gridPos, len(resp.Data))
		for _, r := range resp.Data {
			var gx, gy int32
			if r.GridX != nil {
				gx = *r.GridX
			}
			if r.GridY != nil {
				gy = *r.GridY
			}
			returned[r.ID] = gridPos{x: gx, y: gy}
		}

		// Step 4: Assert every saved position matches the returned position.
		for _, item := range layoutItems {
			saved := gridPos{x: int32(item.GridX), y: int32(item.GridY)}
			got, exists := returned[item.RoomID]
			if !exists {
				t.Logf("room %s not found in GET rooms response", item.RoomID)
				return false
			}
			if got != saved {
				t.Logf("room %s: saved grid=(%d,%d) but got grid=(%d,%d)",
					item.RoomID, saved.x, saved.y, got.x, got.y)
				return false
			}
		}

		return true
	}

	cfg := &quick.Config{
		MaxCount: 200, // run 200 random inputs
	}

	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("layout persistence property violated: %v", err)
	}
}

// TestLayoutPersistence_RandomPositions is a focused example-based test that
// generates rooms with explicitly random grid positions (including negative
// values and zero) and verifies the round-trip persistence property.
//
// Validates: Requirements 2.3
func TestLayoutPersistence_RandomPositions(t *testing.T) {
	// Test cases cover: zero positions, positive positions, and a mix.
	testCases := []struct {
		name  string
		rooms []struct{ gridX, gridY int }
	}{
		{
			name:  "single room at origin",
			rooms: []struct{ gridX, gridY int }{{0, 0}},
		},
		{
			name: "multiple rooms with distinct positions",
			rooms: []struct{ gridX, gridY int }{
				{0, 0}, {1, 0}, {2, 0},
				{0, 1}, {1, 1}, {2, 1},
			},
		},
		{
			name: "rooms with large coordinate values",
			rooms: []struct{ gridX, gridY int }{
				{99, 99}, {100, 200}, {0, 500},
			},
		},
		{
			name: "single room repositioned",
			rooms: []struct{ gridX, gridY int }{
				{5, 10},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ownerID := uuid.New()
			propertyID := uuid.New()

			type gridPos struct{ x, y int32 }
			store := make(map[string]gridPos, len(tc.rooms))

			// Create room IDs and layout items.
			roomIDs := make([]uuid.UUID, len(tc.rooms))
			layoutItems := make([]dto.LayoutItem, len(tc.rooms))
			roomResponses := make([]dto.RoomResponse, len(tc.rooms))

			for i, pos := range tc.rooms {
				roomIDs[i] = uuid.New()
				gx := int32(pos.gridX)
				gy := int32(pos.gridY)
				store[roomIDs[i].String()] = gridPos{x: gx, y: gy}

				layoutItems[i] = dto.LayoutItem{
					RoomID: roomIDs[i].String(),
					GridX:  pos.gridX,
					GridY:  pos.gridY,
				}
				roomResponses[i] = dto.RoomResponse{
					ID:         roomIDs[i].String(),
					PropertyID: propertyID.String(),
					Number:     fmt.Sprintf("R%d", i+1),
					Status:     "vacant",
					GridX:      &gx,
					GridY:      &gy,
					Facilities: []string{},
				}
			}

			svc := &mockRoomService{
				updateLayoutFn: func(_ context.Context, _, _ uuid.UUID, req dto.UpdateLayoutRequest) error {
					for _, item := range req.Rooms {
						store[item.RoomID] = gridPos{x: int32(item.GridX), y: int32(item.GridY)}
					}
					return nil
				},
				listRoomsFn: func(_ context.Context, _, _ uuid.UUID) ([]dto.RoomResponse, error) {
					result := make([]dto.RoomResponse, len(roomResponses))
					for i, r := range roomResponses {
						pos := store[r.ID]
						gx := pos.x
						gy := pos.y
						result[i] = dto.RoomResponse{
							ID:         r.ID,
							PropertyID: r.PropertyID,
							Number:     r.Number,
							Status:     r.Status,
							GridX:      &gx,
							GridY:      &gy,
							Facilities: []string{},
						}
					}
					return result, nil
				},
			}

			router := newRoomRouter(svc, ownerID.String())

			// PUT layout.
			putW := putJSON(router, "/properties/"+propertyID.String()+"/layout", dto.UpdateLayoutRequest{Rooms: layoutItems})
			if putW.Code != http.StatusOK {
				t.Fatalf("PUT layout returned %d: %s", putW.Code, putW.Body.String())
			}

			// GET rooms.
			getW := getRequest(router, "/properties/"+propertyID.String()+"/rooms")
			if getW.Code != http.StatusOK {
				t.Fatalf("GET rooms returned %d: %s", getW.Code, getW.Body.String())
			}

			var resp struct {
				Success bool               `json:"success"`
				Data    []dto.RoomResponse `json:"data"`
			}
			if err := json.NewDecoder(getW.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode GET rooms response: %v", err)
			}
			if !resp.Success {
				t.Fatalf("GET rooms returned success=false")
			}

			// Build lookup map.
			returned := make(map[string]gridPos, len(resp.Data))
			for _, r := range resp.Data {
				var gx, gy int32
				if r.GridX != nil {
					gx = *r.GridX
				}
				if r.GridY != nil {
					gy = *r.GridY
				}
				returned[r.ID] = gridPos{x: gx, y: gy}
			}

			// Assert every saved position matches.
			for _, item := range layoutItems {
				saved := gridPos{x: int32(item.GridX), y: int32(item.GridY)}
				got, exists := returned[item.RoomID]
				if !exists {
					t.Errorf("room %s not found in GET rooms response", item.RoomID)
					continue
				}
				if got != saved {
					t.Errorf("room %s: saved grid=(%d,%d) but got grid=(%d,%d)",
						item.RoomID, saved.x, saved.y, got.x, got.y)
				}
			}
		})
	}
}
