package dto

import (
	"context"
	"time"

	"github.com/flexprice/flexprice/internal/domain/group"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/flexprice/flexprice/internal/validator"
)

// CreateGroupRequest represents the request to create a group
type CreateGroupRequest struct {
	Name       string   `json:"name" validate:"required"`
	EntityType string   `json:"entity_type" validate:"required"`
	PriceIDs   []string `json:"price_ids,omitempty"`
}

func (r *CreateGroupRequest) Validate() error {
	return validator.ValidateRequest(r)
}

func (r *CreateGroupRequest) ToGroup(ctx context.Context) *group.Group {
	return &group.Group{
		ID:            types.GenerateUUIDWithPrefix(types.UUID_PREFIX_GROUP),
		Name:          r.Name,
		EntityType:    r.EntityType,
		EnvironmentID: types.GetEnvironmentID(ctx),
		BaseModel:     types.GetDefaultBaseModel(ctx),
	}
}

// UpdateGroupRequest represents the request to update a group
type UpdateGroupRequest struct {
	Name     *string  `json:"name,omitempty"`
	PriceIDs []string `json:"price_ids,omitempty"`
}

func (r *UpdateGroupRequest) Validate() error {
	return validator.ValidateRequest(r)
}

// GroupResponse represents the group response
type GroupResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	EntityType string    `json:"entity_type"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NewGroupResponse creates a new group response
func NewGroupResponse(group *group.Group, priceIDs []string) *GroupResponse {
	return &GroupResponse{
		ID:         group.ID,
		Name:       group.Name,
		EntityType: group.EntityType,
		Status:     string(group.Status),
		CreatedAt:  group.CreatedAt,
		UpdatedAt:  group.UpdatedAt,
	}
}

// ListGroupsResponse represents the response for listing groups
type ListGroupsResponse struct {
	Groups []*GroupResponse `json:"groups"`
	Total  int              `json:"total"`
}
