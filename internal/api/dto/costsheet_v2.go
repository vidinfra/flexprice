package dto

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/costsheet_v2"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/validator"
)

// CreateCostsheetV2Request represents the request to create a new costsheet v2
type CreateCostsheetV2Request struct {
	Name        string            `json:"name" validate:"required,min=1,max=255"`
	LookupKey   string            `json:"lookup_key,omitempty" validate:"omitempty,min=1,max=255"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Validate validates the create costsheet v2 request
func (r *CreateCostsheetV2Request) Validate() error {
	return validator.ValidateRequest(r)
}

// ToCostsheetV2 converts the request to a domain model
func (r *CreateCostsheetV2Request) ToCostsheetV2(ctx context.Context) *costsheet_v2.CostsheetV2 {
	baseModel := types.GetDefaultBaseModel(ctx)
	return &costsheet_v2.CostsheetV2{
		ID:            types.GenerateUUIDWithPrefix(types.UUID_PREFIX_COSTSHEET_V2),
		Name:          r.Name,
		LookupKey:     r.LookupKey,
		Description:   r.Description,
		Metadata:      r.Metadata,
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     baseModel,
	}
}

// UpdateCostsheetV2Request represents the request to update an existing costsheet v2
type UpdateCostsheetV2Request struct {
	Name        string            `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	LookupKey   string            `json:"lookup_key,omitempty" validate:"omitempty,min=1,max=255"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Validate validates the update costsheet v2 request
func (r *UpdateCostsheetV2Request) Validate() error {
	return validator.ValidateRequest(r)
}

// UpdateCostsheetV2 updates the costsheet v2 with the provided data
func (r *UpdateCostsheetV2Request) UpdateCostsheetV2(costsheet *costsheet_v2.CostsheetV2, ctx context.Context) {
	if r.Name != "" {
		costsheet.Name = r.Name
	}
	if r.LookupKey != "" {
		costsheet.LookupKey = r.LookupKey
	}
	if r.Description != "" {
		costsheet.Description = r.Description
	}
	if r.Metadata != nil {
		costsheet.Metadata = r.Metadata
	}
}

// CostsheetV2Response represents a costsheet v2 in API responses
type CostsheetV2Response struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	LookupKey     string            `json:"lookup_key,omitempty"`
	Description   string            `json:"description,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	TenantID      string            `json:"tenant_id"`
	EnvironmentID string            `json:"environment_id"`
	Status        types.Status      `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	CreatedBy     string            `json:"created_by,omitempty"`
	UpdatedBy     string            `json:"updated_by,omitempty"`
	Prices        []*PriceResponse  `json:"prices,omitempty"` // Associated prices
}

// ToCostsheetV2Response converts a domain model to response DTO
func ToCostsheetV2Response(costsheet *costsheet_v2.CostsheetV2) *CostsheetV2Response {
	return &CostsheetV2Response{
		ID:            costsheet.ID,
		Name:          costsheet.Name,
		LookupKey:     costsheet.LookupKey,
		Description:   costsheet.Description,
		Metadata:      costsheet.Metadata,
		TenantID:      costsheet.TenantID,
		EnvironmentID: costsheet.EnvironmentID,
		Status:        costsheet.Status,
		CreatedAt:     costsheet.CreatedAt,
		UpdatedAt:     costsheet.UpdatedAt,
		CreatedBy:     costsheet.CreatedBy,
		UpdatedBy:     costsheet.UpdatedBy,
		Prices:        nil, // Prices will be populated by the service layer when expanded
	}
}

// ToCostsheetV2ResponseWithPrices converts a domain model to response DTO with prices
func ToCostsheetV2ResponseWithPrices(costsheet *costsheet_v2.CostsheetV2, prices []*PriceResponse) *CostsheetV2Response {
	resp := ToCostsheetV2Response(costsheet)
	resp.Prices = prices
	return resp
}

// ToCostsheetV2 converts response DTO to domain model
func (r *CostsheetV2Response) ToCostsheetV2() *costsheet_v2.CostsheetV2 {
	return &costsheet_v2.CostsheetV2{
		ID:            r.ID,
		Name:          r.Name,
		LookupKey:     r.LookupKey,
		Description:   r.Description,
		Metadata:      r.Metadata,
		EnvironmentID: r.EnvironmentID,
		BaseModel: types.BaseModel{
			TenantID:  r.TenantID,
			Status:    r.Status,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
			CreatedBy: r.CreatedBy,
			UpdatedBy: r.UpdatedBy,
		},
	}
}

// CreateCostsheetV2Response represents the response for creating a costsheet v2
type CreateCostsheetV2Response struct {
	CostsheetV2 *CostsheetV2Response `json:"costsheet_v2"`
}

// ListCostsheetV2Response represents the response for listing costsheet v2 records
type ListCostsheetV2Response struct {
	Items      []*CostsheetV2Response    `json:"items"`
	Pagination *types.PaginationResponse `json:"pagination"`
}

// GetCostsheetV2Response represents the response for getting a single costsheet v2
type GetCostsheetV2Response struct {
	CostsheetV2 *CostsheetV2Response `json:"costsheet_v2"`
}

// UpdateCostsheetV2Response represents the response for updating a costsheet v2
type UpdateCostsheetV2Response struct {
	CostsheetV2 *CostsheetV2Response `json:"costsheet_v2"`
}

// DeleteCostsheetV2Response represents the response for deleting a costsheet v2
type DeleteCostsheetV2Response struct {
	Message string `json:"message"`
	ID      string `json:"id"`
}
