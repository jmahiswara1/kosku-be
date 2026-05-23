package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/repository"
)

// TenantPortalService handles business logic for the tenant self-service portal.
type TenantPortalService struct {
	queries *repository.Queries
}

// NewTenantPortalService creates a new TenantPortalService.
func NewTenantPortalService(queries *repository.Queries) *TenantPortalService {
	return &TenantPortalService{queries: queries}
}

// GetMyRoom returns the tenant's current room details and active contract.
func (s *TenantPortalService) GetMyRoom(ctx context.Context, tenantID uuid.UUID) (dto.TenantRoomResponse, error) {
	// Fetch tenant record (includes property_id and room_id).
	tenant, err := s.queries.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.TenantRoomResponse{}, ErrNotFound
		}
		return dto.TenantRoomResponse{}, fmt.Errorf("get my room: get tenant: %w", err)
	}

	if !tenant.RoomID.Valid {
		return dto.TenantRoomResponse{}, ErrNotFound
	}

	// Fetch room details.
	room, err := s.queries.GetRoom(ctx, tenant.RoomID.UUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.TenantRoomResponse{}, ErrNotFound
		}
		return dto.TenantRoomResponse{}, fmt.Errorf("get my room: get room: %w", err)
	}

	// Fetch active contract.
	contract, err := s.queries.GetActiveContract(ctx, tenantID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return dto.TenantRoomResponse{}, fmt.Errorf("get my room: get contract: %w", err)
	}

	resp := dto.TenantRoomResponse{
		RoomID:     room.ID.String(),
		RoomNumber: room.Number,
		Status:     room.Status,
	}
	if room.Floor.Valid {
		resp.Floor = int(room.Floor.Int32)
	}
	if room.RoomTypeName.Valid {
		resp.RoomType = room.RoomTypeName.String
	}
	if room.MonthlyPrice.Valid {
		resp.MonthlyPrice = room.MonthlyPrice.String
	}
	if tenant.PropertyID.Valid {
		resp.PropertyID = tenant.PropertyID.UUID.String()
	}

	// Include active contract if found.
	if contract.ID != uuid.Nil {
		c := contractToDTO(contract)
		resp.ActiveContract = &c
	}

	return resp, nil
}

// ListMyBills returns all bills for the authenticated tenant.
func (s *TenantPortalService) ListMyBills(ctx context.Context, tenantID uuid.UUID, status string, page, perPage int) ([]dto.BillResponse, int64, error) {
	bills, err := s.queries.ListBillsByTenant(ctx, tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("list my bills: %w", err)
	}

	// Apply status filter in-memory.
	var filtered []repository.Bill
	for _, b := range bills {
		if status == "" || b.Status == status {
			filtered = append(filtered, b)
		}
	}

	total := int64(len(filtered))

	// Apply pagination.
	offset := (page - 1) * perPage
	if int64(offset) >= total {
		return []dto.BillResponse{}, total, nil
	}
	end := offset + perPage
	if int64(end) > total {
		end = int(total)
	}
	filtered = filtered[offset:end]

	result := make([]dto.BillResponse, 0, len(filtered))
	for _, b := range filtered {
		result = append(result, billToDTO(b, ""))
	}
	return result, total, nil
}

// GetBillReceipt generates a PDF receipt for a paid bill.
// Returns ErrNotFound if the bill doesn't exist or doesn't belong to the tenant.
// Returns ErrForbidden if the bill is not paid.
func (s *TenantPortalService) GetBillReceipt(ctx context.Context, tenantID, billID uuid.UUID) ([]byte, string, error) {
	// Fetch the bill.
	bill, err := s.queries.GetBill(ctx, billID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("get bill receipt: get bill: %w", err)
	}

	// Verify the bill belongs to this tenant.
	if bill.TenantID != tenantID {
		return nil, "", ErrForbidden
	}

	// Only paid bills can have receipts.
	if bill.Status != "paid" {
		return nil, "", ErrBillNotPaid
	}

	// Fetch property info.
	prop, err := s.queries.GetProperty(ctx, bill.PropertyID)
	if err != nil {
		return nil, "", fmt.Errorf("get bill receipt: get property: %w", err)
	}

	// Fetch tenant profile.
	tenantProfile, err := s.queries.GetProfile(ctx, tenantID)
	if err != nil {
		return nil, "", fmt.Errorf("get bill receipt: get tenant profile: %w", err)
	}

	// Fetch room info.
	room, err := s.queries.GetRoom(ctx, bill.RoomID)
	if err != nil {
		return nil, "", fmt.Errorf("get bill receipt: get room: %w", err)
	}

	// Fetch utility charges.
	charges, err := s.queries.ListUtilityCharges(ctx, billID)
	if err != nil {
		return nil, "", fmt.Errorf("get bill receipt: list charges: %w", err)
	}

	// Fetch confirmed payment for payment date.
	payments, err := s.queries.ListPaymentsByBill(ctx, billID)
	if err != nil {
		return nil, "", fmt.Errorf("get bill receipt: list payments: %w", err)
	}

	// Find the confirmed payment.
	var paymentDate string
	for _, p := range payments {
		if p.Status == "confirmed" && p.ConfirmedAt.Valid {
			paymentDate = p.ConfirmedAt.Time.Format("2006-01-02")
			break
		}
	}

	// Generate PDF.
	pdfBytes, err := generateReceiptPDF(receiptData{
		Bill:        bill,
		Property:    prop,
		TenantName:  tenantProfile.FullName,
		RoomNumber:  room.Number,
		Charges:     charges,
		PaymentDate: paymentDate,
	})
	if err != nil {
		return nil, "", fmt.Errorf("get bill receipt: generate pdf: %w", err)
	}

	filename := fmt.Sprintf("receipt_%s_%d_%02d.pdf", billID.String()[:8], bill.PeriodYear, bill.PeriodMonth)
	return pdfBytes, filename, nil
}

// ListMyTickets returns all complaint tickets submitted by the tenant.
func (s *TenantPortalService) ListMyTickets(ctx context.Context, tenantID uuid.UUID, status string, page, perPage int) ([]dto.TicketResponse, int64, error) {
	// Fetch all tickets for this tenant using the existing GetTicket approach.
	// We need a query that lists tickets by tenant_id — use a hand-written query.
	tickets, err := s.queries.ListTicketsByTenant(ctx, tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("list my tickets: %w", err)
	}

	// Apply status filter in-memory.
	var filtered []repository.Ticket
	for _, t := range tickets {
		if status == "" || t.Status == status {
			filtered = append(filtered, t)
		}
	}

	total := int64(len(filtered))

	// Apply pagination.
	offset := (page - 1) * perPage
	if int64(offset) >= total {
		return []dto.TicketResponse{}, total, nil
	}
	end := offset + perPage
	if int64(end) > total {
		end = int(total)
	}
	filtered = filtered[offset:end]

	result := make([]dto.TicketResponse, 0, len(filtered))
	for _, t := range filtered {
		result = append(result, ticketToDTO(t, ""))
	}
	return result, total, nil
}

// CreateMyTicket creates a complaint ticket on behalf of the tenant.
// It delegates to the existing TicketService.CreateTicket logic.
func (s *TenantPortalService) CreateMyTicket(
	ctx context.Context,
	tenantID uuid.UUID,
	req dto.CreateTicketRequest,
	photos [][]byte,
	_ []string,
) (dto.TicketResponse, error) {
	// Validate attachment count.
	if len(photos) > maxTicketPhotos {
		return dto.TicketResponse{}, ErrTooManyAttachments
	}

	// Fetch the tenant to get property_id and room_id.
	tenant, err := s.queries.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.TicketResponse{}, ErrNotFound
		}
		return dto.TicketResponse{}, fmt.Errorf("create my ticket: get tenant: %w", err)
	}

	if !tenant.PropertyID.Valid {
		return dto.TicketResponse{}, fmt.Errorf("create my ticket: tenant has no assigned property")
	}

	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}
	switch priority {
	case "low", "medium", "high", "urgent":
	default:
		priority = "medium"
	}

	var roomID uuid.NullUUID
	if tenant.RoomID.Valid {
		roomID = uuid.NullUUID{UUID: tenant.RoomID.UUID, Valid: true}
	}

	ticket, err := s.queries.CreateTicket(ctx, repository.CreateTicketParams{
		TenantID:    tenantID,
		PropertyID:  tenant.PropertyID.UUID,
		RoomID:      roomID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    priority,
		Status:      "open",
	})
	if err != nil {
		return dto.TicketResponse{}, fmt.Errorf("create my ticket: insert ticket: %w", err)
	}

	// Fetch the owner of the property to send notification.
	prop, err := s.queries.GetProperty(ctx, tenant.PropertyID.UUID)
	if err == nil {
		notifBody := fmt.Sprintf("New complaint from tenant: %s", req.Title)
		_, _ = s.queries.CreateNotification(ctx, repository.CreateNotificationParams{
			UserID:   prop.OwnerID,
			Type:     "ticket_created",
			Title:    "New Complaint Ticket",
			Body:     sql.NullString{String: notifBody, Valid: true},
			EntityID: uuid.NullUUID{UUID: ticket.ID, Valid: true},
		})
	}

	resp := ticketToDTO(ticket, "")
	return resp, nil
}

// ListMyContracts returns all contracts for the authenticated tenant.
func (s *TenantPortalService) ListMyContracts(ctx context.Context, tenantID uuid.UUID) ([]dto.ContractResponse, error) {
	contracts, err := s.queries.ListContractsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list my contracts: %w", err)
	}

	result := make([]dto.ContractResponse, 0, len(contracts))
	for _, c := range contracts {
		result = append(result, contractToDTO(c))
	}
	return result, nil
}

// RequestContractRenewal creates a notification for the owner with the renewal request.
func (s *TenantPortalService) RequestContractRenewal(ctx context.Context, tenantID uuid.UUID, req dto.ContractRenewalRequest) error {
	// Fetch the active contract.
	contract, err := s.queries.GetActiveContract(ctx, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoActiveContract
		}
		return fmt.Errorf("request renewal: get active contract: %w", err)
	}

	// Fetch tenant profile.
	tenantProfile, err := s.queries.GetProfile(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("request renewal: get tenant profile: %w", err)
	}

	// Fetch property to get owner_id.
	prop, err := s.queries.GetProperty(ctx, contract.PropertyID)
	if err != nil {
		return fmt.Errorf("request renewal: get property: %w", err)
	}

	// Build notification body.
	notifTitle := "Contract Renewal Request"
	notifBody := fmt.Sprintf(
		"Tenant %s has requested a contract renewal for room %s. Current contract ends: %s.",
		tenantProfile.FullName,
		contract.RoomID.String(),
		contract.EndDate.Format("2006-01-02"),
	)
	if req.RequestedEndDate != "" {
		notifBody += fmt.Sprintf(" Requested new end date: %s.", req.RequestedEndDate)
	}
	if req.Notes != "" {
		notifBody += fmt.Sprintf(" Notes: %s", req.Notes)
	}

	// Create notification for the owner.
	_, err = s.queries.CreateNotification(ctx, repository.CreateNotificationParams{
		UserID:   prop.OwnerID,
		Type:     "contract_renewal_request",
		Title:    notifTitle,
		Body:     sql.NullString{String: notifBody, Valid: true},
		EntityID: uuid.NullUUID{UUID: contract.ID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("request renewal: create notification: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(tenantID, "request_contract_renewal", "contract", contract.ID, map[string]string{
		"tenant_id":   tenantID.String(),
		"contract_id": contract.ID.String(),
		"property_id": contract.PropertyID.String(),
	}))

	return nil
}

//  PDF generation ─

// ErrBillNotPaid is returned when a receipt is requested for an unpaid bill.
var ErrBillNotPaid = errors.New("receipt is only available for paid bills")

// receiptData holds all data needed to generate a receipt PDF.
type receiptData struct {
	Bill        repository.Bill
	Property    repository.GetPropertyRow
	TenantName  string
	RoomNumber  string
	Charges     []repository.UtilityCharge
	PaymentDate string
}

// generateReceiptPDF creates a PDF receipt and returns the bytes.
func generateReceiptPDF(data receiptData) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetMargins(20, 20, 20)

	// ── Header
	pdf.SetFont("Helvetica", "B", 18)
	pdf.CellFormat(0, 10, "PAYMENT RECEIPT", "", 1, "C", false, 0, "")

	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(0, 6, data.Property.Name, "", 1, "C", false, 0, "")
	if data.Property.Address != "" {
		pdf.CellFormat(0, 6, data.Property.Address, "", 1, "C", false, 0, "")
	}
	if data.Property.Phone.Valid {
		pdf.CellFormat(0, 6, "Tel: "+data.Property.Phone.String, "", 1, "C", false, 0, "")
	}

	pdf.Ln(4)
	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(4)

	// ── Receipt details
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(50, 7, "Receipt No:", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(0, 7, data.Bill.ID.String()[:8], "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(50, 7, "Tenant Name:", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(0, 7, data.TenantName, "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(50, 7, "Room Number:", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(0, 7, data.RoomNumber, "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(50, 7, "Bill Period:", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	period := fmt.Sprintf("%s %d", time.Month(data.Bill.PeriodMonth).String(), data.Bill.PeriodYear)
	pdf.CellFormat(0, 7, period, "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(50, 7, "Due Date:", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	pdf.CellFormat(0, 7, data.Bill.DueDate.Format("2006-01-02"), "", 1, "L", false, 0, "")

	if data.PaymentDate != "" {
		pdf.SetFont("Helvetica", "B", 11)
		pdf.CellFormat(50, 7, "Payment Date:", "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 11)
		pdf.CellFormat(0, 7, data.PaymentDate, "", 1, "L", false, 0, "")
	}

	pdf.Ln(4)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(4)

	// ── Charges breakdown ─
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 7, "Charges Breakdown", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	// Table header.
	pdf.SetFillColor(230, 230, 230)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(100, 7, "Description", "1", 0, "L", true, 0, "")
	pdf.CellFormat(70, 7, "Amount (IDR)", "1", 1, "R", true, 0, "")

	// Base rent row.
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(100, 7, "Base Rent", "1", 0, "L", false, 0, "")
	pdf.CellFormat(70, 7, formatAmount(data.Bill.BaseAmount), "1", 1, "R", false, 0, "")

	// Utility charges.
	for _, charge := range data.Charges {
		label := "Utility - " + charge.Type
		if charge.Note.Valid && charge.Note.String != "" {
			label += " (" + charge.Note.String + ")"
		}
		pdf.CellFormat(100, 7, label, "1", 0, "L", false, 0, "")
		pdf.CellFormat(70, 7, formatAmount(charge.Amount), "1", 1, "R", false, 0, "")
	}

	// Penalty if any.
	if data.Bill.PenaltyAmount.Valid && data.Bill.PenaltyAmount.String != "" && data.Bill.PenaltyAmount.String != "0.00" {
		pdf.CellFormat(100, 7, "Late Payment Penalty", "1", 0, "L", false, 0, "")
		pdf.CellFormat(70, 7, formatAmount(data.Bill.PenaltyAmount.String), "1", 1, "R", false, 0, "")
	}

	// Total row.
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(200, 220, 255)
	totalAmount := data.Bill.BaseAmount
	if data.Bill.TotalAmount.Valid {
		totalAmount = data.Bill.TotalAmount.String
	}
	pdf.CellFormat(100, 8, "TOTAL", "1", 0, "L", true, 0, "")
	pdf.CellFormat(70, 8, formatAmount(totalAmount), "1", 1, "R", true, 0, "")

	pdf.Ln(6)
	pdf.Line(20, pdf.GetY(), 190, pdf.GetY())
	pdf.Ln(4)

	// ── Status stamp
	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(0, 128, 0)
	pdf.CellFormat(0, 10, "STATUS: PAID", "", 1, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	pdf.Ln(4)
	pdf.SetFont("Helvetica", "I", 9)
	pdf.CellFormat(0, 6, fmt.Sprintf("Generated on %s", time.Now().Format("2006-01-02 15:04:05")), "", 1, "C", false, 0, "")

	// Write to buffer.
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// formatAmount formats a numeric string as a human-readable amount.
func formatAmount(s string) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	// Format with thousands separator.
	return fmt.Sprintf("%.2f", f)
}
