package group

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

// Group represents a grouping entity for organizing related items
type Group struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	EntityType    string `json:"entity_type"`
	EnvironmentID string `json:"environment_id"`
	types.BaseModel
}

// Repository defines the interface for group data operations
type Repository interface {
	Create(ctx context.Context, group *Group) error
	Get(ctx context.Context, id string) (*Group, error)
	GetByName(ctx context.Context, name string) (*Group, error)
	Update(ctx context.Context, group *Group) error
	Delete(ctx context.Context, id string) error
	GetPricesInGroup(ctx context.Context, groupID string) ([]string, error)
	UpdatePriceGroup(ctx context.Context, priceID string, groupID *string) error
	ValidatePricesExist(ctx context.Context, priceIDs []string) error
	ValidatePricesNotInOtherGroup(ctx context.Context, priceIDs []string, excludeGroupID string) error
}
