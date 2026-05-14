package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/middleware"
	"github.com/kosku/backend/internal/service"
	"github.com/xuri/excelize/v2"
)

// BillingServicer is the interface that BillingHandler depends on.
type BillingServicer interface {
	GenerateBills(ctx context.Context, ownerID uuid.UUID, req dto.GenerateBillsRequest) ([]dto.BillResponse, error)
	ListBills(ctx context.Context, ownerID uuid.UUID, propertyID uuid.UUID, status, tenantName, fromDate, toDate string, page, perPage int) ([]dto.BillResponse, int64, error)
	GetBill(ctx context.Context, ownerID, billID uuid.UUID) (dto.BillResponse, error)
	UpdateUtilities(ctx context.Context, ownerID, billID uuid.UUID, req dto.UpdateUtilitiesRequest) (dto.BillResponse, error)
	SubmitPayment(ctx context.Context, tenantID uuid.UUID, req dto.SubmitPaymentRequest, fileData []byte, declaredContentType string) (dto.PaymentResponse, error)
	ConfirmPayment(ctx context.Context, ownerID, paymentID uuid.UUID) (dto.PaymentResponse, error)
	RejectPayment(ctx context.Context, ownerID, paymentID uuid.UUID, req dto.RejectPaymentRequest) (dto.PaymentResponse, error)
	GetFinancialReport(ctx context.Context, ownerID, propertyID uuid.UUID, fromMonth, fromYear, toMonth, toYear int) (dto.FinancialReportResponse, error)
}

// Ensure *service.BillingService satisfies BillingServicer at compile time.
var _ BillingServicer = (*service.BillingService)(nil)

// BillingHandler holds the dependencies for billing-related HTTP handlers.
type BillingHandler struct {
	svc BillingServicer
}

// NewBillingHandler creates a new BillingHandler.
func NewBillingHandler(svc *service.BillingService) *BillingHandler {
	return &BillingHandler{svc: svc}
}

// NewBillingHandlerWithService creates a new BillingHandler with any BillingServicer.
// Intended for use in tests.
func NewBillingHandlerWithService(svc BillingServicer) *BillingHandler {
	return &BillingHandler{svc: svc}
}

// GenerateBills handles POST /v1/bills/generate.
// Generates one bill per active contract in the property for the given period.
//
//	@Summary		Generate monthly bills
//	@Description	Generates one bill per active contract in the property for the specified billing period.
//	@Tags			billing
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.GenerateBillsRequest	true	"Bill generation parameters"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/bills/generate [post]
//	@Security		BearerAuth
func (h *BillingHandler) GenerateBills(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	var req dto.GenerateBillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	bills, err := h.svc.GenerateBills(c.Request.Context(), ownerID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("GENERATE_BILLS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    bills,
		"meta":    gin.H{"generated": len(bills)},
	})
}

// ListBills handles GET /v1/bills.
// Returns bills with optional filters and pagination.
//
//	@Summary		List bills
//	@Description	Returns bills for a property with optional filters (status, date range, tenant name) and pagination.
//	@Tags			billing
//	@Produce		json
//	@Param			property_id		query	string	true	"Property UUID"
//	@Param			status			query	string	false	"Filter by status (unpaid, pending, paid, overdue)"
//	@Param			tenant_name		query	string	false	"Filter by tenant name (partial match)"
//	@Param			from_date		query	string	false	"Filter by due date from (YYYY-MM-DD)"
//	@Param			to_date			query	string	false	"Filter by due date to (YYYY-MM-DD)"
//	@Param			page			query	int		false	"Page number (default 1)"
//	@Param			per_page		query	int		false	"Items per page (default 20)"
//	@Success		200				{object}	map[string]interface{}
//	@Failure		400				{object}	map[string]interface{}
//	@Failure		403				{object}	map[string]interface{}
//	@Router			/bills [get]
//	@Security		BearerAuth
func (h *BillingHandler) ListBills(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	propertyIDStr := c.Query("property_id")
	if propertyIDStr == "" {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", "property_id is required"))
		return
	}
	propertyID, err := uuid.Parse(propertyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property_id"))
		return
	}

	page := 1
	perPage := 20
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := c.Query("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}

	bills, total, err := h.svc.ListBills(
		c.Request.Context(),
		ownerID,
		propertyID,
		c.Query("status"),
		c.Query("tenant_name"),
		c.Query("from_date"),
		c.Query("to_date"),
		page,
		perPage,
	)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("LIST_BILLS_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    bills,
		"meta": gin.H{
			"page":     page,
			"per_page": perPage,
			"total":    total,
		},
	})
}

// GetBill handles GET /v1/bills/:id.
// Returns a single bill with its utility charges.
//
//	@Summary		Get bill detail
//	@Description	Returns a single bill including all utility charges.
//	@Tags			billing
//	@Produce		json
//	@Param			id	path		string	true	"Bill UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		400	{object}	map[string]interface{}
//	@Failure		403	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Router			/bills/{id} [get]
//	@Security		BearerAuth
func (h *BillingHandler) GetBill(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	billID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid bill ID"))
		return
	}

	bill, err := h.svc.GetBill(c.Request.Context(), ownerID, billID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Bill not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this bill's property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("GET_BILL_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(bill))
}

// UpdateUtilities handles PUT /v1/bills/:id/utilities.
// Replaces all utility charges for a bill and recalculates utility_amount.
//
//	@Summary		Update utility charges
//	@Description	Replaces all utility charges for a bill and recalculates the utility_amount.
//	@Tags			billing
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"Bill UUID"
//	@Param			body	body		dto.UpdateUtilitiesRequest	true	"Utility charges"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/bills/{id}/utilities [put]
//	@Security		BearerAuth
func (h *BillingHandler) UpdateUtilities(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	billID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid bill ID"))
		return
	}

	var req dto.UpdateUtilitiesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	bill, err := h.svc.UpdateUtilities(c.Request.Context(), ownerID, billID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Bill not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this bill's property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("UPDATE_UTILITIES_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(bill))
}

// SubmitPayment handles POST /v1/payments.
// Validates proof image, uploads to storage, inserts payment row, sets bill to pending.
//
//	@Summary		Submit payment
//	@Description	Validates the proof image, uploads it to storage, inserts a payment row, and sets the bill status to pending.
//	@Tags			payments
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			bill_id	formData	string	true	"Bill UUID"
//	@Param			amount	formData	number	true	"Payment amount"
//	@Param			proof	formData	file	true	"Transfer proof image (JPEG, PNG, or WebP; max 5MB)"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Router			/payments [post]
//	@Security		BearerAuth
func (h *BillingHandler) SubmitPayment(c *gin.Context) {
	tenantIDStr := c.GetString(middleware.ContextKeyUserID)
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, errorResponse("INVALID_TOKEN", "Invalid user ID in token"))
		return
	}

	// Parse multipart form.
	if err := c.Request.ParseMultipartForm(maxUploadMemory); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_FORM", "Failed to parse multipart form"))
		return
	}

	var req dto.SubmitPaymentRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	file, header, err := c.Request.FormFile("proof")
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("MISSING_FILE", "A 'proof' file field is required"))
		return
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("READ_FILE_ERROR", "Failed to read uploaded file"))
		return
	}

	declaredContentType := header.Header.Get("Content-Type")

	payment, err := h.svc.SubmitPayment(c.Request.Context(), tenantID, req, fileData, declaredContentType)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Bill not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "This bill does not belong to you"))
			return
		}
		if errors.Is(err, service.ErrBillAlreadyPaid) {
			c.JSON(http.StatusConflict, errorResponse("BILL_ALREADY_PAID", "This bill is already paid"))
			return
		}
		if errors.Is(err, service.ErrFileTooLarge) {
			c.JSON(http.StatusBadRequest, errorResponse("FILE_TOO_LARGE", err.Error()))
			return
		}
		if errors.Is(err, service.ErrInvalidFileType) {
			c.JSON(http.StatusBadRequest, errorResponse("INVALID_FILE_TYPE", err.Error()))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("SUBMIT_PAYMENT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusCreated, successResponse(payment))
}

// ConfirmPayment handles PUT /v1/payments/:id/confirm.
// Confirms a payment, marks the bill as paid, and sends a confirmation email.
//
//	@Summary		Confirm payment
//	@Description	Confirms a payment, marks the bill as paid, and sends a confirmation email to the tenant.
//	@Tags			payments
//	@Produce		json
//	@Param			id	path		string	true	"Payment UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		400	{object}	map[string]interface{}
//	@Failure		403	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]interface{}
//	@Failure		409	{object}	map[string]interface{}
//	@Router			/payments/{id}/confirm [put]
//	@Security		BearerAuth
func (h *BillingHandler) ConfirmPayment(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	paymentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid payment ID"))
		return
	}

	payment, err := h.svc.ConfirmPayment(c.Request.Context(), ownerID, paymentID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Payment not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this payment's property"))
			return
		}
		if errors.Is(err, service.ErrPaymentAlreadyConfirmed) {
			c.JSON(http.StatusConflict, errorResponse("PAYMENT_ALREADY_CONFIRMED", "Payment is already confirmed"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("CONFIRM_PAYMENT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(payment))
}

// RejectPayment handles PUT /v1/payments/:id/reject.
// Rejects a payment, resets the bill to unpaid, and sends a rejection email.
//
//	@Summary		Reject payment
//	@Description	Rejects a payment, resets the bill status to unpaid, and sends a rejection email to the tenant.
//	@Tags			payments
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"Payment UUID"
//	@Param			body	body		dto.RejectPaymentRequest	true	"Rejection reason"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]interface{}
//	@Failure		403		{object}	map[string]interface{}
//	@Failure		404		{object}	map[string]interface{}
//	@Failure		409		{object}	map[string]interface{}
//	@Router			/payments/{id}/reject [put]
//	@Security		BearerAuth
func (h *BillingHandler) RejectPayment(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	paymentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid payment ID"))
		return
	}

	var req dto.RejectPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", err.Error()))
		return
	}

	payment, err := h.svc.RejectPayment(c.Request.Context(), ownerID, paymentID, req)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Payment not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this payment's property"))
			return
		}
		if errors.Is(err, service.ErrPaymentAlreadyConfirmed) {
			c.JSON(http.StatusConflict, errorResponse("PAYMENT_ALREADY_CONFIRMED", "Cannot reject a confirmed payment"))
			return
		}
		if errors.Is(err, service.ErrPaymentAlreadyRejected) {
			c.JSON(http.StatusConflict, errorResponse("PAYMENT_ALREADY_REJECTED", "Payment is already rejected"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("REJECT_PAYMENT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(payment))
}

// GetFinancialReport handles GET /v1/reports/financial.
// Returns aggregated financial data for a property and date range.
//
//	@Summary		Get financial report
//	@Description	Returns aggregated income data for a property and date range.
//	@Tags			reports
//	@Produce		json
//	@Param			property_id		query	string	true	"Property UUID"
//	@Param			from_month		query	int		true	"From month (1-12)"
//	@Param			from_year		query	int		true	"From year"
//	@Param			to_month		query	int		true	"To month (1-12)"
//	@Param			to_year			query	int		true	"To year"
//	@Success		200				{object}	map[string]interface{}
//	@Failure		400				{object}	map[string]interface{}
//	@Failure		403				{object}	map[string]interface{}
//	@Router			/reports/financial [get]
//	@Security		BearerAuth
func (h *BillingHandler) GetFinancialReport(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	propertyIDStr := c.Query("property_id")
	if propertyIDStr == "" {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", "property_id is required"))
		return
	}
	propertyID, err := uuid.Parse(propertyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property_id"))
		return
	}

	fromMonth, _ := strconv.Atoi(c.Query("from_month"))
	fromYear, _ := strconv.Atoi(c.Query("from_year"))
	toMonth, _ := strconv.Atoi(c.Query("to_month"))
	toYear, _ := strconv.Atoi(c.Query("to_year"))

	if fromMonth < 1 || fromMonth > 12 || toMonth < 1 || toMonth > 12 || fromYear < 2000 || toYear < 2000 {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", "Invalid date range parameters"))
		return
	}

	report, err := h.svc.GetFinancialReport(c.Request.Context(), ownerID, propertyID, fromMonth, fromYear, toMonth, toYear)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("FINANCIAL_REPORT_ERROR", err.Error()))
		return
	}

	c.JSON(http.StatusOK, successResponse(report))
}

// ExportFinancialReport handles GET /v1/reports/financial/export.
// Generates an Excel file with the financial report and streams it as a download.
//
//	@Summary		Export financial report
//	@Description	Generates an Excel file with the financial report and streams it as a download.
//	@Tags			reports
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Param			property_id		query	string	true	"Property UUID"
//	@Param			from_month		query	int		true	"From month (1-12)"
//	@Param			from_year		query	int		true	"From year"
//	@Param			to_month		query	int		true	"To month (1-12)"
//	@Param			to_year			query	int		true	"To year"
//	@Success		200				{file}	binary
//	@Failure		400				{object}	map[string]interface{}
//	@Failure		403				{object}	map[string]interface{}
//	@Router			/reports/financial/export [get]
//	@Security		BearerAuth
func (h *BillingHandler) ExportFinancialReport(c *gin.Context) {
	ownerID, ok := ownerIDFromContext(c)
	if !ok {
		return
	}

	propertyIDStr := c.Query("property_id")
	if propertyIDStr == "" {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", "property_id is required"))
		return
	}
	propertyID, err := uuid.Parse(propertyIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("INVALID_ID", "Invalid property_id"))
		return
	}

	fromMonth, _ := strconv.Atoi(c.Query("from_month"))
	fromYear, _ := strconv.Atoi(c.Query("from_year"))
	toMonth, _ := strconv.Atoi(c.Query("to_month"))
	toYear, _ := strconv.Atoi(c.Query("to_year"))

	if fromMonth < 1 || fromMonth > 12 || toMonth < 1 || toMonth > 12 || fromYear < 2000 || toYear < 2000 {
		c.JSON(http.StatusBadRequest, errorResponse("VALIDATION_ERROR", "Invalid date range parameters"))
		return
	}

	report, err := h.svc.GetFinancialReport(c.Request.Context(), ownerID, propertyID, fromMonth, fromYear, toMonth, toYear)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			c.JSON(http.StatusNotFound, errorResponse("NOT_FOUND", "Property not found"))
			return
		}
		if errors.Is(err, service.ErrForbidden) {
			c.JSON(http.StatusForbidden, errorResponse("FORBIDDEN", "You do not own this property"))
			return
		}
		c.JSON(http.StatusInternalServerError, errorResponse("FINANCIAL_REPORT_ERROR", err.Error()))
		return
	}

	// Generate Excel file.
	buf, err := generateFinancialReportExcel(report)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse("EXPORT_ERROR", "Failed to generate Excel file"))
		return
	}

	filename := fmt.Sprintf("financial_report_%d%02d_%d%02d.xlsx", fromYear, fromMonth, toYear, toMonth)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Length", strconv.Itoa(buf.Len()))
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, buf)
}

// generateFinancialReportExcel creates an Excel workbook from the financial report data.
func generateFinancialReportExcel(report dto.FinancialReportResponse) (*bytes.Buffer, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Financial Report"
	f.SetSheetName("Sheet1", sheet)

	// Header row.
	headers := []string{"Period", "Total Billed", "Total Paid", "Bill Count"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Data rows.
	for i, row := range report.Rows {
		rowNum := i + 2
		period := fmt.Sprintf("%d/%02d", row.PeriodYear, row.PeriodMonth)
		f.SetCellValue(sheet, mustCell(1, rowNum), period)
		f.SetCellValue(sheet, mustCell(2, rowNum), row.TotalBilled)
		f.SetCellValue(sheet, mustCell(3, rowNum), row.TotalPaid)
		f.SetCellValue(sheet, mustCell(4, rowNum), row.BillCount)
	}

	// Summary row.
	summaryRow := len(report.Rows) + 3
	f.SetCellValue(sheet, mustCell(1, summaryRow), fmt.Sprintf("Generated: %s", time.Now().Format("2006-01-02 15:04:05")))

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return &buf, nil
}

// mustCell converts column/row coordinates to a cell name, ignoring errors.
func mustCell(col, row int) string {
	cell, _ := excelize.CoordinatesToCellName(col, row)
	return cell
}
