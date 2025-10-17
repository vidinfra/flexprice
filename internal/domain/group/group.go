package group

import (
	"context"

	"github.com/flexprice/flexprice/internal/types"
)

// Group represents a grouping entity for organizing related items
type Group struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	EntityType    types.GroupEntityType `json:"entity_type"`
	EnvironmentID string                `json:"environment_id"`
	LookupKey     string                `json:"lookup_key,omitempty"`
	types.BaseModel
}

// Repository defines the interface for group data operations
type Repository interface {
	Create(ctx context.Context, group *Group) error
	Get(ctx context.Context, id string) (*Group, error)
	GetByName(ctx context.Context, name string) (*Group, error)
	GetByLookupKey(ctx context.Context, lookupKey string) (*Group, error)
	Update(ctx context.Context, group *Group) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter *types.GroupFilter) ([]*Group, error)
}
