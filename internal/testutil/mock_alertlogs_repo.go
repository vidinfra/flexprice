package testutil

import (
	"context"

	"github.com/flexprice/flexprice/internal/domain/alertlogs"
	"github.com/flexprice/flexprice/internal/types"
)

// MockAlertLogsRepo is a simple mock that implements alertlogs.Repository
type MockAlertLogsRepo struct{}

// NewMockAlertLogsRepo creates a new mock alert logs repository
func NewMockAlertLogsRepo() *MockAlertLogsRepo {
	return &MockAlertLogsRepo{}
}

// Create does nothing (for testing wallet operations that don't need real alerts)
func (m *MockAlertLogsRepo) Create(ctx context.Context, alertLog *alertlogs.AlertLog) error {
	return nil
}

// Get returns nil (not needed for wallet tests)
func (m *MockAlertLogsRepo) Get(ctx context.Context, id string) (*alertlogs.AlertLog, error) {
	return nil, nil
}

// List returns empty slice (not needed for wallet tests)
func (m *MockAlertLogsRepo) List(ctx context.Context, filter *types.AlertLogFilter) ([]*alertlogs.AlertLog, error) {
	return []*alertlogs.AlertLog{}, nil
}

// Count returns 0 (not needed for wallet tests)
func (m *MockAlertLogsRepo) Count(ctx context.Context, filter *types.AlertLogFilter) (int, error) {
	return 0, nil
}

// GetLatestByEntity returns nil (no previous alerts for wallet tests)
func (m *MockAlertLogsRepo) GetLatestByEntity(ctx context.Context, entityType types.AlertEntityType, entityID string) (*alertlogs.AlertLog, error) {
	return nil, nil
}

// GetLatestByEntityAndAlertType returns nil (no previous alerts for wallet tests)
func (m *MockAlertLogsRepo) GetLatestByEntityAndAlertType(ctx context.Context, entityType types.AlertEntityType, entityID string, alertType types.AlertType) (*alertlogs.AlertLog, error) {
	return nil, nil
}

// ListByEntity returns empty slice (not needed for wallet tests)
func (m *MockAlertLogsRepo) ListByEntity(ctx context.Context, entityType types.AlertEntityType, entityID string, limit int) ([]*alertlogs.AlertLog, error) {
	return []*alertlogs.AlertLog{}, nil
}

// ListByAlertType returns empty slice (not needed for wallet tests)
func (m *MockAlertLogsRepo) ListByAlertType(ctx context.Context, alertType types.AlertType, limit int) ([]*alertlogs.AlertLog, error) {
	return []*alertlogs.AlertLog{}, nil
}
