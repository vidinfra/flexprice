package chargebee

import (
	"context"

	itemFamilyAction "github.com/chargebee/chargebee-go/v3/actions/itemfamily"
	"github.com/chargebee/chargebee-go/v3/models/itemfamily"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
)

// ChargebeeItemFamilyService defines the interface for Chargebee item family operations
type ChargebeeItemFamilyService interface {
	CreateItemFamily(ctx context.Context, req *ItemFamilyCreateRequest) (*ItemFamilyResponse, error)
	ListItemFamilies(ctx context.Context) ([]*ItemFamilyResponse, error)
	GetLatestItemFamily(ctx context.Context) (*ItemFamilyResponse, error)
}

// ItemFamilyService handles Chargebee item family operations
type ItemFamilyService struct {
	client ChargebeeClient
	logger *logger.Logger
}

// NewItemFamilyService creates a new Chargebee item family service
func NewItemFamilyService(
	client ChargebeeClient,
	logger *logger.Logger,
) ChargebeeItemFamilyService {
	return &ItemFamilyService{
		client: client,
		logger: logger,
	}
}

// CreateItemFamily creates a new item family in Chargebee
func (s *ItemFamilyService) CreateItemFamily(ctx context.Context, req *ItemFamilyCreateRequest) (*ItemFamilyResponse, error) {
	// Initialize Chargebee SDK
	if err := s.client.(*Client).InitializeChargebeeSDK(ctx); err != nil {
		return nil, err
	}

	s.logger.Infow("creating item family in Chargebee",
		"family_id", req.ID,
		"name", req.Name)

	// Prepare request params
	createParams := &itemfamily.CreateRequestParams{
		Id:   req.ID,
		Name: req.Name,
	}

	if req.Description != "" {
		createParams.Description = req.Description
	}

	// Create item family
	result, err := itemFamilyAction.Create(createParams).Request()
	if err != nil {
		s.logger.Errorw("failed to create item family in Chargebee",
			"family_id", req.ID,
			"error", err)
		return nil, ierr.NewError("failed to create item family in Chargebee").
			WithReportableDetails(map[string]interface{}{
				"error":     err.Error(),
				"family_id": req.ID,
			}).
			WithHint("Check Chargebee API credentials and item family data").
			Mark(ierr.ErrValidation)
	}

	itemFamily := result.ItemFamily

	s.logger.Infow("successfully created item family in Chargebee",
		"family_id", itemFamily.Id,
		"name", itemFamily.Name)

	// Convert to our DTO format
	familyResponse := &ItemFamilyResponse{
		ID:              itemFamily.Id,
		Name:            itemFamily.Name,
		Description:     itemFamily.Description,
		Status:          string(itemFamily.Status),
		ResourceVersion: itemFamily.ResourceVersion,
		UpdatedAt:       timestampToTime(itemFamily.UpdatedAt),
	}

	return familyResponse, nil
}

// ListItemFamilies retrieves all item families from Chargebee
func (s *ItemFamilyService) ListItemFamilies(ctx context.Context) ([]*ItemFamilyResponse, error) {
	// Initialize Chargebee SDK
	if err := s.client.(*Client).InitializeChargebeeSDK(ctx); err != nil {
		return nil, err
	}

	s.logger.Infow("listing item families from Chargebee")

	// List all item families
	result, err := itemFamilyAction.List(&itemfamily.ListRequestParams{
		Limit: intPtr(100), // Get up to 100 families
	}).ListRequest()

	if err != nil {
		s.logger.Errorw("failed to list item families from Chargebee",
			"error", err)
		return nil, ierr.NewError("failed to list item families from Chargebee").
			WithReportableDetails(map[string]interface{}{
				"error": err.Error(),
			}).
			WithHint("Check Chargebee API credentials").
			Mark(ierr.ErrValidation)
	}

	// Convert to our DTO format
	families := make([]*ItemFamilyResponse, 0, len(result.List))
	for _, entry := range result.List {
		itemFamily := entry.ItemFamily
		families = append(families, &ItemFamilyResponse{
			ID:              itemFamily.Id,
			Name:            itemFamily.Name,
			Description:     itemFamily.Description,
			Status:          string(itemFamily.Status),
			ResourceVersion: itemFamily.ResourceVersion,
			UpdatedAt:       timestampToTime(itemFamily.UpdatedAt),
		})
	}

	s.logger.Infow("successfully listed item families from Chargebee",
		"count", len(families))

	return families, nil
}

// GetLatestItemFamily retrieves the most recently updated item family
func (s *ItemFamilyService) GetLatestItemFamily(ctx context.Context) (*ItemFamilyResponse, error) {
	families, err := s.ListItemFamilies(ctx)
	if err != nil {
		return nil, err
	}

	if len(families) == 0 {
		return nil, ierr.NewError("no item families found in Chargebee").
			WithHint("Please create an item family first").
			Mark(ierr.ErrNotFound)
	}

	// Find the latest family by UpdatedAt
	latest := families[0]
	for _, family := range families[1:] {
		if family.UpdatedAt.After(latest.UpdatedAt) {
			latest = family
		}
	}

	s.logger.Infow("found latest item family",
		"family_id", latest.ID,
		"name", latest.Name,
		"updated_at", latest.UpdatedAt)

	return latest, nil
}
