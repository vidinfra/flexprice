package service

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
)

// GroupService interface defines the business logic for group management
type GroupService interface {
	CreateGroup(ctx context.Context, req dto.CreateGroupRequest) (*dto.GroupResponse, error)
	GetGroup(ctx context.Context, id string) (*dto.GroupResponse, error)
	UpdateGroup(ctx context.Context, id string, req dto.UpdateGroupRequest) (*dto.GroupResponse, error)
	DeleteGroup(ctx context.Context, id string) error
}

type groupService struct {
	ServiceParams
}

func NewGroupService(params ServiceParams) GroupService {
	return &groupService{
		ServiceParams: params,
	}
}

// CreateGroup creates a new group with associated prices
func (s *groupService) CreateGroup(ctx context.Context, req dto.CreateGroupRequest) (*dto.GroupResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Check if group name already exists
	existingGroup, err := s.GroupRepo.GetByName(ctx, req.Name)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if existingGroup != nil {
		return nil, ierr.NewError("group name already exists").
			WithHint("A group with this name already exists").
			Mark(ierr.ErrValidation)
	}

	// Validate price IDs if provided
	if len(req.PriceIDs) > 0 {
		if err := s.GroupRepo.ValidatePricesExist(ctx, req.PriceIDs); err != nil {
			return nil, err
		}

		// Check if any prices are already in other groups
		if err := s.GroupRepo.ValidatePricesNotInOtherGroup(ctx, req.PriceIDs, ""); err != nil {
			return nil, err
		}
	}

	// Create group
	group := req.ToGroup(ctx)
	if err := s.GroupRepo.Create(ctx, group); err != nil {
		return nil, err
	}

	// Associate prices with group
	if len(req.PriceIDs) > 0 {
		for _, priceID := range req.PriceIDs {
			if err := s.GroupRepo.UpdatePriceGroup(ctx, priceID, &group.ID); err != nil {
				return nil, err
			}
		}
	}

	return dto.NewGroupResponse(group, req.PriceIDs), nil
}

// GetGroup retrieves a group by ID
func (s *groupService) GetGroup(ctx context.Context, id string) (*dto.GroupResponse, error) {
	group, err := s.GroupRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Get associated prices
	priceIDs, err := s.GroupRepo.GetPricesInGroup(ctx, id)
	if err != nil {
		return nil, err
	}

	return dto.NewGroupResponse(group, priceIDs), nil
}

// UpdateGroup updates a group's details and associated prices
func (s *groupService) UpdateGroup(ctx context.Context, id string, req dto.UpdateGroupRequest) (*dto.GroupResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	group, err := s.GroupRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update name if provided
	if req.Name != nil {
		// Check if new name already exists
		existingGroup, err := s.GroupRepo.GetByName(ctx, *req.Name)
		if err != nil && !ent.IsNotFound(err) {
			return nil, err
		}
		if existingGroup != nil && existingGroup.ID != id {
			return nil, ierr.NewError("group name already exists").
				WithHint("A group with this name already exists").
				Mark(ierr.ErrValidation)
		}
		group.Name = *req.Name
	}

	// Update group
	if err := s.GroupRepo.Update(ctx, group); err != nil {
		return nil, err
	}

	// Update associated prices if provided
	if req.PriceIDs != nil {
		// Validate new price IDs
		if err := s.GroupRepo.ValidatePricesExist(ctx, req.PriceIDs); err != nil {
			return nil, err
		}

		// Check if any prices are already in other groups
		if err := s.GroupRepo.ValidatePricesNotInOtherGroup(ctx, req.PriceIDs, id); err != nil {
			return nil, err
		}

		// First, remove all existing associations
		existingPriceIDs, err := s.GroupRepo.GetPricesInGroup(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, priceID := range existingPriceIDs {
			if err := s.GroupRepo.UpdatePriceGroup(ctx, priceID, nil); err != nil {
				return nil, err
			}
		}

		// Then, add new associations
		for _, priceID := range req.PriceIDs {
			if err := s.GroupRepo.UpdatePriceGroup(ctx, priceID, &id); err != nil {
				return nil, err
			}
		}
	}

	// Get updated price IDs
	priceIDs, err := s.GroupRepo.GetPricesInGroup(ctx, id)
	if err != nil {
		return nil, err
	}

	return dto.NewGroupResponse(group, priceIDs), nil
}

// DeleteGroup deletes a group and removes all price associations
func (s *groupService) DeleteGroup(ctx context.Context, id string) error {
	// Remove group associations from all prices
	priceIDs, err := s.GroupRepo.GetPricesInGroup(ctx, id)
	if err != nil {
		return err
	}

	for _, priceID := range priceIDs {
		if err := s.GroupRepo.UpdatePriceGroup(ctx, priceID, nil); err != nil {
			return err
		}
	}

	// Soft delete the group
	return s.GroupRepo.Delete(ctx, id)
}
