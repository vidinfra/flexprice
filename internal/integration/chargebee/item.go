package chargebee

import (
	"context"

	itemAction "github.com/chargebee/chargebee-go/v3/actions/item"
	"github.com/chargebee/chargebee-go/v3/models/item"
	itemEnum "github.com/chargebee/chargebee-go/v3/models/item/enum"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
)

// ChargebeeItemService defines the interface for Chargebee item operations
type ChargebeeItemService interface {
	CreateItem(ctx context.Context, req *ItemCreateRequest) (*ItemResponse, error)
	RetrieveItem(ctx context.Context, itemID string) (*ItemResponse, error)
}

// ItemService handles Chargebee item operations
type ItemService struct {
	client ChargebeeClient
	logger *logger.Logger
}

// NewItemService creates a new Chargebee item service
func NewItemService(
	client ChargebeeClient,
	logger *logger.Logger,
) ChargebeeItemService {
	return &ItemService{
		client: client,
		logger: logger,
	}
}

// CreateItem creates a new item in Chargebee
func (s *ItemService) CreateItem(ctx context.Context, req *ItemCreateRequest) (*ItemResponse, error) {
	// Initialize Chargebee SDK
	if err := s.client.(*Client).InitializeChargebeeSDK(ctx); err != nil {
		return nil, err
	}

	s.logger.Infow("creating item in Chargebee",
		"item_id", req.ID,
		"name", req.Name,
		"type", req.Type,
		"item_family_id", req.ItemFamilyID)

	// Prepare request params
	createParams := &item.CreateRequestParams{
		Id:              req.ID,
		Name:            req.Name,
		Type:            itemEnum.Type(req.Type),
		ItemFamilyId:    req.ItemFamilyID,
		EnabledInPortal: boolPtr(req.EnabledInPortal),
	}

	if req.Description != "" {
		createParams.Description = req.Description
	}

	if req.ExternalName != "" {
		createParams.ExternalName = req.ExternalName
	}

	// Create item
	result, err := itemAction.Create(createParams).Request()
	if err != nil {
		s.logger.Errorw("failed to create item in Chargebee",
			"item_id", req.ID,
			"error", err)
		return nil, ierr.NewError("failed to create item in Chargebee").
			WithReportableDetails(map[string]interface{}{
				"error":   err.Error(),
				"item_id": req.ID,
			}).
			WithHint("Check Chargebee API credentials and item data").
			Mark(ierr.ErrValidation)
	}

	itemData := result.Item

	s.logger.Infow("successfully created item in Chargebee",
		"item_id", itemData.Id,
		"name", itemData.Name,
		"type", itemData.Type)

	// Convert to our DTO format
	itemResponse := &ItemResponse{
		ID:              itemData.Id,
		Name:            itemData.Name,
		Type:            string(itemData.Type),
		ItemFamilyID:    itemData.ItemFamilyId,
		Description:     itemData.Description,
		ExternalName:    itemData.ExternalName,
		Status:          string(itemData.Status),
		ResourceVersion: itemData.ResourceVersion,
		UpdatedAt:       timestampToTime(itemData.UpdatedAt),
	}

	return itemResponse, nil
}


// RetrieveItem retrieves an item from Chargebee
func (s *ItemService) RetrieveItem(ctx context.Context, itemID string) (*ItemResponse, error) {
	// Initialize Chargebee SDK
	if err := s.client.(*Client).InitializeChargebeeSDK(ctx); err != nil {
		return nil, err
	}

	s.logger.Infow("retrieving item from Chargebee",
		"item_id", itemID)

	// Retrieve item
	result, err := itemAction.Retrieve(itemID).Request()
	if err != nil {
		s.logger.Errorw("failed to retrieve item from Chargebee",
			"item_id", itemID,
			"error", err)
		return nil, ierr.NewError("failed to retrieve item from Chargebee").
			WithReportableDetails(map[string]interface{}{
				"error":   err.Error(),
				"item_id": itemID,
			}).
			WithHint("Check if item exists in Chargebee").
			Mark(ierr.ErrNotFound)
	}

	itemData := result.Item

	s.logger.Infow("successfully retrieved item from Chargebee",
		"item_id", itemData.Id,
		"name", itemData.Name)

	// Convert to our DTO format
	itemResponse := &ItemResponse{
		ID:              itemData.Id,
		Name:            itemData.Name,
		Type:            string(itemData.Type),
		ItemFamilyID:    itemData.ItemFamilyId,
		Description:     itemData.Description,
		ExternalName:    itemData.ExternalName,
		Status:          string(itemData.Status),
		ResourceVersion: itemData.ResourceVersion,
		UpdatedAt:       timestampToTime(itemData.UpdatedAt),
	}

	return itemResponse, nil
}


