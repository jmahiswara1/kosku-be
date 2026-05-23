// Package service contains business logic for the KosKu API.
package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf"
	"github.com/kosku/backend/internal/dto"
	"github.com/kosku/backend/internal/repository"
	"github.com/kosku/backend/pkg/storage"
)

const documentsBucket = "documents"

// ContractTemplateService handles business logic for contract templates and generation.
type ContractTemplateService struct {
	queries       *repository.Queries
	storageClient *storage.Client
}

// NewContractTemplateService creates a new ContractTemplateService.
func NewContractTemplateService(queries *repository.Queries, storageClient *storage.Client) *ContractTemplateService {
	return &ContractTemplateService{
		queries:       queries,
		storageClient: storageClient,
	}
}

// ListTemplates returns all contract templates owned by the given owner.
func (s *ContractTemplateService) ListTemplates(ctx context.Context, ownerID uuid.UUID) ([]dto.ContractTemplateResponse, error) {
	rows, err := s.queries.ListContractTemplates(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list contract templates: %w", err)
	}

	result := make([]dto.ContractTemplateResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, contractTemplateToDTO(row))
	}
	return result, nil
}

// CreateTemplate creates a new contract template for the given owner.
func (s *ContractTemplateService) CreateTemplate(ctx context.Context, ownerID uuid.UUID, req dto.CreateContractTemplateRequest) (dto.ContractTemplateResponse, error) {
	tmpl, err := s.queries.CreateContractTemplate(ctx, repository.CreateContractTemplateParams{
		OwnerID: ownerID,
		Name:    req.Name,
		Content: req.Content,
	})
	if err != nil {
		return dto.ContractTemplateResponse{}, fmt.Errorf("create contract template: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "create_contract_template", "contract_template", tmpl.ID, map[string]string{
		"template_id": tmpl.ID.String(),
		"name":        tmpl.Name,
	}))

	return contractTemplateToDTO(tmpl), nil
}

// UpdateTemplate updates an existing contract template. Only the owner may update their templates.
func (s *ContractTemplateService) UpdateTemplate(ctx context.Context, ownerID, templateID uuid.UUID, req dto.UpdateContractTemplateRequest) (dto.ContractTemplateResponse, error) {
	// Fetch existing to verify ownership.
	existing, err := s.queries.GetContractTemplate(ctx, templateID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.ContractTemplateResponse{}, ErrNotFound
		}
		return dto.ContractTemplateResponse{}, fmt.Errorf("update contract template: get: %w", err)
	}
	if existing.OwnerID != ownerID {
		return dto.ContractTemplateResponse{}, ErrForbidden
	}

	updated, err := s.queries.UpdateContractTemplate(ctx, repository.UpdateContractTemplateParams{
		ID:      templateID,
		Name:    req.Name,
		Content: req.Content,
	})
	if err != nil {
		return dto.ContractTemplateResponse{}, fmt.Errorf("update contract template: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "update_contract_template", "contract_template", templateID, map[string]string{
		"template_id": templateID.String(),
		"name":        req.Name,
	}))

	return contractTemplateToDTO(updated), nil
}

// GenerateContract populates a template with tenant/contract data, generates PDF and DOCX
// files, uploads them to the documents bucket, and returns the download URLs.
func (s *ContractTemplateService) GenerateContract(ctx context.Context, ownerID, templateID uuid.UUID, req dto.GenerateContractRequest) (dto.GenerateContractResponse, error) {
	// Fetch and verify template ownership.
	tmpl, err := s.queries.GetContractTemplate(ctx, templateID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.GenerateContractResponse{}, ErrNotFound
		}
		return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: get template: %w", err)
	}
	if tmpl.OwnerID != ownerID {
		return dto.GenerateContractResponse{}, ErrForbidden
	}

	// Resolve the contract record.
	var contract repository.Contract
	if req.ContractID != "" {
		contractID, err := uuid.Parse(req.ContractID)
		if err != nil {
			return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: invalid contract_id: %w", err)
		}
		contract, err = s.queries.GetContractByID(ctx, contractID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return dto.GenerateContractResponse{}, ErrNotFound
			}
			return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: get contract: %w", err)
		}
	} else if req.TenantID != "" {
		tenantID, err := uuid.Parse(req.TenantID)
		if err != nil {
			return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: invalid tenant_id: %w", err)
		}
		contract, err = s.queries.GetActiveContract(ctx, tenantID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return dto.GenerateContractResponse{}, ErrNotFound
			}
			return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: get active contract: %w", err)
		}
	} else {
		return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: either contract_id or tenant_id is required")
	}

	// Verify the contract belongs to a property owned by this owner.
	prop, err := s.queries.GetProperty(ctx, contract.PropertyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.GenerateContractResponse{}, ErrNotFound
		}
		return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: get property: %w", err)
	}
	if prop.OwnerID != ownerID {
		return dto.GenerateContractResponse{}, ErrForbidden
	}

	// Fetch tenant profile.
	tenant, err := s.queries.GetTenant(ctx, contract.TenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.GenerateContractResponse{}, ErrNotFound
		}
		return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: get tenant: %w", err)
	}

	// Fetch room.
	room, err := s.queries.GetRoom(ctx, contract.RoomID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.GenerateContractResponse{}, ErrNotFound
		}
		return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: get room: %w", err)
	}

	// Fetch owner profile.
	ownerProfile, err := s.queries.GetProfile(ctx, ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dto.GenerateContractResponse{}, ErrNotFound
		}
		return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: get owner profile: %w", err)
	}

	// Build placeholder values.
	placeholders := map[string]string{
		"{{tenant_name}}":   tenant.FullName,
		"{{room_number}}":   room.Number,
		"{{start_date}}":    contract.StartDate.Format("2006-01-02"),
		"{{end_date}}":      contract.EndDate.Format("2006-01-02"),
		"{{monthly_price}}": contract.MonthlyPrice,
		"{{owner_name}}":    ownerProfile.FullName,
		"{{property_name}}": prop.Name,
	}

	// Populate template content.
	populated := populatePlaceholders(tmpl.Content, placeholders)

	// Generate PDF.
	pdfData, err := generateContractPDF(populated, tmpl.Name)
	if err != nil {
		return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: generate PDF: %w", err)
	}

	// Generate DOCX.
	docxData, err := generateContractDOCX(populated)
	if err != nil {
		return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: generate DOCX: %w", err)
	}

	// Upload PDF.
	pdfFilename := fmt.Sprintf("contracts/%s_%s.pdf", templateID.String(), uuid.New().String())
	pdfURL, err := s.storageClient.UploadFile(ctx, documentsBucket, pdfFilename, pdfData, "application/pdf")
	if err != nil {
		return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: upload PDF: %w", err)
	}

	// Upload DOCX.
	docxFilename := fmt.Sprintf("contracts/%s_%s.docx", templateID.String(), uuid.New().String())
	docxURL, err := s.storageClient.UploadFile(ctx, documentsBucket, docxFilename, docxData, "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err != nil {
		// Clean up the already-uploaded PDF on failure.
		_ = s.storageClient.DeleteFile(ctx, documentsBucket, pdfFilename)
		return dto.GenerateContractResponse{}, fmt.Errorf("generate contract: upload DOCX: %w", err)
	}

	// Write audit log — non-fatal.
	_, _ = s.queries.CreateAuditLog(ctx, auditLogParams(ownerID, "generate_contract", "contract_template", templateID, map[string]string{
		"template_id": templateID.String(),
		"contract_id": contract.ID.String(),
		"tenant_id":   contract.TenantID.String(),
	}))

	return dto.GenerateContractResponse{
		TemplateID: templateID.String(),
		ContractID: contract.ID.String(),
		PDFURL:     pdfURL,
		DOCXURL:    docxURL,
	}, nil
}

//  helpers

// populatePlaceholders replaces all placeholder keys in content with their values.
func populatePlaceholders(content string, placeholders map[string]string) string {
	result := content
	for placeholder, value := range placeholders {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// generateContractPDF creates a PDF document from the given text content using gofpdf.
func generateContractPDF(content, title string) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(title, false)
	pdf.SetAuthor("KosKu", false)
	pdf.AddPage()

	// Write title.
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, title, "", 1, "C", false, 0, "")
	pdf.Ln(4)

	// Write body content — split on newlines and write each line.
	pdf.SetFont("Helvetica", "", 11)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Use MultiCell to handle long lines with word wrap.
		pdf.MultiCell(0, 6, line, "", "L", false)
	}

	// Add generation timestamp at the bottom.
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "I", 9)
	pdf.CellFormat(0, 6, fmt.Sprintf("Generated by KosKu on %s", time.Now().Format("2006-01-02 15:04:05")), "", 1, "R", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdf output: %w", err)
	}
	return buf.Bytes(), nil
}

// generateContractDOCX creates a DOCX document from the given text content.
// It produces a minimal but valid DOCX (Office Open XML) file by constructing
// the ZIP archive manually — no external DOCX library required.
func generateContractDOCX(content string) ([]byte, error) {
	return buildMinimalDOCX(content)
}

// contractTemplateToDTO converts a repository.ContractTemplate to dto.ContractTemplateResponse.
func contractTemplateToDTO(t repository.ContractTemplate) dto.ContractTemplateResponse {
	resp := dto.ContractTemplateResponse{
		ID:      t.ID.String(),
		OwnerID: t.OwnerID.String(),
		Name:    t.Name,
		Content: t.Content,
	}
	if t.CreatedAt.Valid {
		resp.CreatedAt = t.CreatedAt.Time.Format(time.RFC3339)
	}
	if t.UpdatedAt.Valid {
		resp.UpdatedAt = t.UpdatedAt.Time.Format(time.RFC3339)
	}
	return resp
}
