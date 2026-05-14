package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// ListTicketsFilteredParams holds the filter parameters for ListTicketsFiltered.
type ListTicketsFilteredParams struct {
	PropertyID uuid.UUID
	Status     string // empty string = no filter
	Priority   string // empty string = no filter
	Limit      int32
	Offset     int32
}

// ListTicketsFilteredRow is the return type for ListTicketsFiltered.
type ListTicketsFilteredRow struct {
	Ticket
	TenantName string `json:"tenant_name"`
}

// ListTicketsFiltered returns tickets for a property with optional status/priority
// filters and pagination. It joins with profiles to include the tenant name.
func (q *Queries) ListTicketsFiltered(ctx context.Context, arg ListTicketsFilteredParams) ([]ListTicketsFilteredRow, int64, error) {
	baseQuery := `
FROM tickets t
JOIN profiles p ON p.id = t.tenant_id
WHERE t.property_id = $1`

	args := []interface{}{arg.PropertyID}
	idx := 2

	if arg.Status != "" {
		baseQuery += fmt.Sprintf(" AND t.status = $%d", idx)
		args = append(args, arg.Status)
		idx++
	}
	if arg.Priority != "" {
		baseQuery += fmt.Sprintf(" AND t.priority = $%d", idx)
		args = append(args, arg.Priority)
		idx++
	}

	// Count query.
	countQuery := "SELECT COUNT(*) " + baseQuery
	var total int64
	if err := q.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data query.
	dataQuery := `SELECT t.id, t.tenant_id, t.property_id, t.room_id, t.title, t.description,
       t.priority, t.status, t.resolution, t.created_at, t.updated_at,
       p.full_name AS tenant_name ` + baseQuery
	dataQuery += " ORDER BY t.created_at DESC"
	dataQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, arg.Limit, arg.Offset)

	rows, err := q.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []ListTicketsFilteredRow
	for rows.Next() {
		var i ListTicketsFilteredRow
		if err := rows.Scan(
			&i.ID,
			&i.TenantID,
			&i.PropertyID,
			&i.RoomID,
			&i.Title,
			&i.Description,
			&i.Priority,
			&i.Status,
			&i.Resolution,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.TenantName,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, 0, err
	}
	return items, total, rows.Err()
}
