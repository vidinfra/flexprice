package chargebee

import (
	"context"

	"github.com/chargebee/chargebee-go/v3/enum"
	itemPriceAction "github.com/chargebee/chargebee-go/v3/actions/itemprice"
	"github.com/chargebee/chargebee-go/v3/models/itemprice"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
)

// ChargebeeItemPriceService defines the interface for Chargebee item price operations
type ChargebeeItemPriceService interface {
	CreateItemPrice(ctx context.Context, req *ItemPriceCreateRequest) (*ItemPriceResponse, error)
	RetrieveItemPrice(ctx context.Context, itemPriceID string) (*ItemPriceResponse, error)
}

// ItemPriceService handles Chargebee item price operations
type ItemPriceService struct {
	client ChargebeeClient
	logger *logger.Logger
}

// NewItemPriceService creates a new Chargebee item price service
func NewItemPriceService(
	client ChargebeeClient,
	logger *logger.Logger,
) ChargebeeItemPriceService {
	return &ItemPriceService{
		client: client,
		logger: logger,
	}
}

// CreateItemPrice creates a new item price in Chargebee
func (s *ItemPriceService) CreateItemPrice(ctx context.Context, req *ItemPriceCreateRequest) (*ItemPriceResponse, error) {
	// Initialize Chargebee SDK
	if err := s.client.(*Client).InitializeChargebeeSDK(ctx); err != nil {
		return nil, err
	}

	s.logger.Infow("creating item price in Chargebee",
		"item_price_id", req.ID,
		"item_id", req.ItemID,
		"price", req.Price,
		"currency", req.CurrencyCode)

	// Prepare request params
	createParams := &itemprice.CreateRequestParams{
		Id:           req.ID,
		ItemId:       req.ItemID,
		Name:         req.Name,
		PricingModel: enum.PricingModel(req.PricingModel),
		Price:        int64Ptr(req.Price),
		CurrencyCode: req.CurrencyCode,
	}

	if req.ExternalName != "" {
		createParams.ExternalName = req.ExternalName
	}

	if req.Description != "" {
		createParams.Description = req.Description
	}

	// Create item price
	result, err := itemPriceAction.Create(createParams).Request()
	if err != nil {
		s.logger.Errorw("failed to create item price in Chargebee",
			"item_price_id", req.ID,
			"item_id", req.ItemID,
			"error", err)
		return nil, ierr.NewError("failed to create item price in Chargebee").
			WithReportableDetails(map[string]interface{}{
				"error":         err.Error(),
				"item_price_id": req.ID,
				"item_id":       req.ItemID,
			}).
			WithHint("Check Chargebee API credentials and item price data").
			Mark(ierr.ErrValidation)
	}

	itemPrice := result.ItemPrice

	s.logger.Infow("successfully created item price in Chargebee",
		"item_price_id", itemPrice.Id,
		"item_id", itemPrice.ItemId,
		"price", itemPrice.Price,
		"currency", itemPrice.CurrencyCode)

	// Convert to our DTO format
	itemPriceResponse := &ItemPriceResponse{
		ID:              itemPrice.Id,
		ItemID:          itemPrice.ItemId,
		Name:            itemPrice.Name,
		ExternalName:    itemPrice.ExternalName,
		PricingModel:    string(itemPrice.PricingModel),
		Price:           itemPrice.Price,
		CurrencyCode:    itemPrice.CurrencyCode,
		Description:     itemPrice.Description,
		Status:          string(itemPrice.Status),
		ResourceVersion: itemPrice.ResourceVersion,
		UpdatedAt:       timestampToTime(itemPrice.UpdatedAt),
	}

	return itemPriceResponse, nil
}


// RetrieveItemPrice retrieves an item price from Chargebee
func (s *ItemPriceService) RetrieveItemPrice(ctx context.Context, itemPriceID string) (*ItemPriceResponse, error) {
	// Initialize Chargebee SDK
	if err := s.client.(*Client).InitializeChargebeeSDK(ctx); err != nil {
		return nil, err
	}

	s.logger.Infow("retrieving item price from Chargebee",
		"item_price_id", itemPriceID)

	// Retrieve item price
	result, err := itemPriceAction.Retrieve(itemPriceID).Request()
	if err != nil {
		s.logger.Errorw("failed to retrieve item price from Chargebee",
			"item_price_id", itemPriceID,
			"error", err)
		return nil, ierr.NewError("failed to retrieve item price from Chargebee").
			WithReportableDetails(map[string]interface{}{
				"error":         err.Error(),
				"item_price_id": itemPriceID,
			}).
			WithHint("Check if item price exists in Chargebee").
			Mark(ierr.ErrNotFound)
	}

	itemPrice := result.ItemPrice

	s.logger.Infow("successfully retrieved item price from Chargebee",
		"item_price_id", itemPrice.Id,
		"item_id", itemPrice.ItemId)

	// Convert to our DTO format
	itemPriceResponse := &ItemPriceResponse{
		ID:              itemPrice.Id,
		ItemID:          itemPrice.ItemId,
		Name:            itemPrice.Name,
		ExternalName:    itemPrice.ExternalName,
		PricingModel:    string(itemPrice.PricingModel),
		Price:           itemPrice.Price,
		CurrencyCode:    itemPrice.CurrencyCode,
		Description:     itemPrice.Description,
		Status:          string(itemPrice.Status),
		ResourceVersion: itemPrice.ResourceVersion,
		UpdatedAt:       timestampToTime(itemPrice.UpdatedAt),
	}

	return itemPriceResponse, nil
}


