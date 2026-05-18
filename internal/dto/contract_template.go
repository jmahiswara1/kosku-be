// Package dto contains data transfer objects for the KosKu API.
package dto

// CreateContractTemplateRequest is the request body for POST /v1/contract-templates.
type CreateContractTemplateRequest struct {
	Name    string `json:"name"    binding:"required"`
	Content string `json:"content" binding:"required"`
}

// UpdateContractTemplateRequest is the request body for PUT /v1/contract-templates/:id.
type UpdateContractTemplateRequest struct {
	Name    string `json:"name"    binding:"required"`
	Content string `json:"content" binding:"required"`
}

// ContractTemplateResponse is the response body for contract template endpoints.
type ContractTemplateResponse struct {
	ID        string `json:"id"`
	OwnerID   string `json:"owner_id"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// GenerateContractRequest is the request body for POST /v1/contract-templates/:id/generate.
// It accepts either a contract_id (to look up an existing contract) or a tenant_id
// (to look up the tenant's active contract).
type GenerateContractRequest struct {
	// ContractID is the UUID of an existing contract record. Mutually exclusive with TenantID.
	ContractID string `json:"contract_id"`
	// TenantID is the UUID of the tenant whose active contract should be used.
	// Used when contract_id is not provided.
	TenantID string `json:"tenant_id"`
}

// GenerateContractResponse is the response body for the contract generation endpoint.
type GenerateContractResponse struct {
	TemplateID string `json:"template_id"`
	ContractID string `json:"contract_id"`
	PDFURL     string `json:"pdf_url"`
	DOCXURL    string `json:"docx_url"`
}
