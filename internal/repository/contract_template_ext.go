// Package repository contains hand-written extensions to the sqlc-generated code.
package repository

import (
	"context"

	"github.com/google/uuid"
)

// GetContractByID fetches a single contract by its primary key.
// This supplements the sqlc-generated code which only has GetActiveContract (by tenant).
func (q *Queries) GetContractByID(ctx context.Context, id uuid.UUID) (Contract, error) {
	const query = `
SELECT id, tenant_id, room_id, property_id, start_date, end_date,
       monthly_price, deposit_amount, deposit_refunded, status,
       file_url, created_at, updated_at
FROM contracts
WHERE id = $1`

	row := q.db.QueryRowContext(ctx, query, id)
	var i Contract
	err := row.Scan(
		&i.ID,
		&i.TenantID,
		&i.RoomID,
		&i.PropertyID,
		&i.StartDate,
		&i.EndDate,
		&i.MonthlyPrice,
		&i.DepositAmount,
		&i.DepositRefunded,
		&i.Status,
		&i.FileUrl,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	if err != nil {
		return Contract{}, err
	}
	return i, nil
}
