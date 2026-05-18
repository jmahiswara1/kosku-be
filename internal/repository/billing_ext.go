// Package repository contains hand-written extensions to the sqlc-generated code.
// These functions supplement the generated queries with more complex operations
// that are difficult to express in sqlc's query format.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ListBillsFilteredParams holds the filter parameters for ListBillsFiltered.
type ListBillsFilteredParams struct {
	PropertyID uuid.UUID
	Status     string    // empty string = no filter
	FromDate   time.Time // zero value = no filter
	ToDate     time.Time // zero value = no filter
	TenantName string    // empty string = no filter
	Limit      int32
	Offset     int32
}

// ListBillsFilteredRow is the return type for ListBillsFiltered.
type ListBillsFilteredRow struct {
	Bill
	TenantName string `json:"tenant_name"`
}

// ListBillsFiltered returns bills for a property with optional filters and pagination.
// It joins with profiles to include the tenant name.
func (q *Queries) ListBillsFiltered(ctx context.Context, arg ListBillsFilteredParams) ([]ListBillsFilteredRow, error) {
	query := `
SELECT b.id, b.tenant_id, b.property_id, b.room_id, b.period_month, b.period_year,
       b.base_amount, b.utility_amount, b.penalty_amount, b.total_amount,
       b.due_date, b.status, b.created_at, b.updated_at,
       p.full_name AS tenant_name
FROM bills b
JOIN profiles p ON p.id = b.tenant_id
WHERE b.property_id = $1`

	args := []interface{}{arg.PropertyID}
	idx := 2

	if arg.Status != "" {
		query += fmt.Sprintf(" AND b.status = $%d", idx)
		args = append(args, arg.Status)
		idx++
	}
	if !arg.FromDate.IsZero() {
		query += fmt.Sprintf(" AND b.due_date >= $%d", idx)
		args = append(args, arg.FromDate)
		idx++
	}
	if !arg.ToDate.IsZero() {
		query += fmt.Sprintf(" AND b.due_date <= $%d", idx)
		args = append(args, arg.ToDate)
		idx++
	}
	if arg.TenantName != "" {
		query += fmt.Sprintf(" AND p.full_name ILIKE $%d", idx)
		args = append(args, "%"+strings.TrimSpace(arg.TenantName)+"%")
		idx++
	}

	query += " ORDER BY b.period_year DESC, b.period_month DESC, b.created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, arg.Limit, arg.Offset)

	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []ListBillsFilteredRow
	for rows.Next() {
		var i ListBillsFilteredRow
		if err := rows.Scan(
			&i.ID,
			&i.TenantID,
			&i.PropertyID,
			&i.RoomID,
			&i.PeriodMonth,
			&i.PeriodYear,
			&i.BaseAmount,
			&i.UtilityAmount,
			&i.PenaltyAmount,
			&i.TotalAmount,
			&i.DueDate,
			&i.Status,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.TenantName,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return items, rows.Err()
}

// CountBillsFiltered returns the total count of bills matching the given filters.
func (q *Queries) CountBillsFiltered(ctx context.Context, arg ListBillsFilteredParams) (int64, error) {
	query := `
SELECT COUNT(*)
FROM bills b
JOIN profiles p ON p.id = b.tenant_id
WHERE b.property_id = $1`

	args := []interface{}{arg.PropertyID}
	idx := 2

	if arg.Status != "" {
		query += fmt.Sprintf(" AND b.status = $%d", idx)
		args = append(args, arg.Status)
		idx++
	}
	if !arg.FromDate.IsZero() {
		query += fmt.Sprintf(" AND b.due_date >= $%d", idx)
		args = append(args, arg.FromDate)
		idx++
	}
	if !arg.ToDate.IsZero() {
		query += fmt.Sprintf(" AND b.due_date <= $%d", idx)
		args = append(args, arg.ToDate)
		idx++
	}
	if arg.TenantName != "" {
		query += fmt.Sprintf(" AND p.full_name ILIKE $%d", idx)
		args = append(args, "%"+strings.TrimSpace(arg.TenantName)+"%")
	}

	var count int64
	err := q.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

// ListActiveContractsByProperty returns all active contracts for a property.
func (q *Queries) ListActiveContractsByProperty(ctx context.Context, propertyID uuid.UUID) ([]Contract, error) {
	const query = `
SELECT id, tenant_id, room_id, property_id, start_date, end_date,
       monthly_price, deposit_amount, deposit_refunded, status,
       file_url, created_at, updated_at
FROM contracts
WHERE property_id = $1
  AND status = 'active'
ORDER BY created_at ASC`

	rows, err := q.db.QueryContext(ctx, query, propertyID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []Contract
	for rows.Next() {
		var i Contract
		if err := rows.Scan(
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
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return items, rows.Err()
}

// UpdateContractDepositRefundedParams holds the parameters for UpdateContractDepositRefunded.
type UpdateContractDepositRefundedParams struct {
	ID              uuid.UUID      `json:"id"`
	DepositRefunded sql.NullString `json:"deposit_refunded"`
}

// UpdateContractDepositRefunded updates the deposit_refunded field on a contract.
func (q *Queries) UpdateContractDepositRefunded(ctx context.Context, arg UpdateContractDepositRefundedParams) (Contract, error) {
	const query = `
UPDATE contracts
SET deposit_refunded = $2,
    updated_at       = NOW()
WHERE id = $1
RETURNING id, tenant_id, room_id, property_id, start_date, end_date, monthly_price, deposit_amount, deposit_refunded, status, file_url, created_at, updated_at`

	row := q.db.QueryRowContext(ctx, query, arg.ID, arg.DepositRefunded)
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
	return i, err
}

// DeleteUtilityChargesByBill deletes all utility charges for a given bill.
func (q *Queries) DeleteUtilityChargesByBill(ctx context.Context, billID uuid.UUID) error {
	const query = `DELETE FROM utility_charges WHERE bill_id = $1`
	_, err := q.db.ExecContext(ctx, query, billID)
	return err
}
