package ent

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/group"
	"github.com/flexprice/flexprice/ent/predicate"
	"github.com/flexprice/flexprice/ent/price"
	"github.com/flexprice/flexprice/internal/cache"
	domainGroup "github.com/flexprice/flexprice/internal/domain/group"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
)

type groupRepository struct {
	client postgres.IClient
	log    *logger.Logger
	cache  cache.Cache
}

func NewGroupRepository(client postgres.IClient, log *logger.Logger, cache cache.Cache) domainGroup.Repository {
	return &groupRepository{
		client: client,
		log:    log,
		cache:  cache,
	}
}

func (r *groupRepository) Create(ctx context.Context, grp *domainGroup.Group) error {
	client := r.client.Querier(ctx)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	_, err := client.Group.Create().
		SetID(grp.ID).
		SetName(grp.Name).
		SetEntityType(grp.EntityType).
		SetTenantID(tenantID).
		SetEnvironmentID(environmentID).
		SetStatus(string(grp.Status)).
		SetCreatedBy(types.GetUserID(ctx)).
		SetUpdatedBy(types.GetUserID(ctx)).
		Save(ctx)

	if err != nil {
		r.log.Error("Failed to create group", "error", err, "group_id", grp.ID)
		return ierr.WithError(err).
			WithHint("Failed to create group").
			Mark(ierr.ErrDatabase)
	}

	return nil
}

func (r *groupRepository) Get(ctx context.Context, id string) (*domainGroup.Group, error) {
	client := r.client.Querier(ctx)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	entGroup, err := client.Group.Query().
		Where(
			group.IDEQ(id),
			group.TenantIDEQ(tenantID),
			group.EnvironmentIDEQ(environmentID),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ierr.WithError(err).
				WithHint("Group not found").
				Mark(ierr.ErrNotFound)
		}
		r.log.Error("Failed to get group", "error", err, "group_id", id)
		return nil, ierr.WithError(err).
			WithHint("Failed to get group").
			Mark(ierr.ErrDatabase)
	}

	return r.toDomainGroup(entGroup), nil
}

func (r *groupRepository) GetByName(ctx context.Context, name string) (*domainGroup.Group, error) {
	client := r.client.Querier(ctx)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	entGroup, err := client.Group.Query().
		Where(
			group.NameEQ(name),
			group.TenantIDEQ(tenantID),
			group.EnvironmentIDEQ(environmentID),
			group.StatusEQ(string(types.StatusPublished)),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil // Return nil instead of error for "not found"
		}
		r.log.Error("Failed to get group by name", "error", err, "name", name)
		return nil, ierr.WithError(err).
			WithHint("Failed to get group by name").
			Mark(ierr.ErrDatabase)
	}

	return r.toDomainGroup(entGroup), nil
}

func (r *groupRepository) List(ctx context.Context, filter *types.GroupFilter) ([]*domainGroup.Group, error) {
	client := r.client.Querier(ctx)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	query := client.Group.Query().
		Where(
			group.TenantIDEQ(tenantID),
			group.EnvironmentIDEQ(environmentID),
		)

	// Apply filters
	if filter.EntityType != "" {
		query = query.Where(group.EntityTypeEQ(filter.EntityType))
	}
	if filter.Name != "" {
		query = query.Where(group.NameContains(filter.Name))
	}

	// Apply pagination
	if filter.QueryFilter != nil {
		if filter.Limit != nil && *filter.Limit > 0 {
			query = query.Limit(*filter.Limit)
		}
		if filter.Offset != nil && *filter.Offset > 0 {
			query = query.Offset(*filter.Offset)
		}
	}

	entGroups, err := query.All(ctx)
	if err != nil {
		r.log.Error("Failed to list groups", "error", err)
		return nil, ierr.WithError(err).
			WithHint("Failed to list groups").
			Mark(ierr.ErrDatabase)
	}

	groups := make([]*domainGroup.Group, len(entGroups))
	for i, entGroup := range entGroups {
		groups[i] = r.toDomainGroup(entGroup)
	}

	return groups, nil
}

func (r *groupRepository) Update(ctx context.Context, grp *domainGroup.Group) error {
	client := r.client.Querier(ctx)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	_, err := client.Group.UpdateOneID(grp.ID).
		Where(
			group.TenantIDEQ(tenantID),
			group.EnvironmentIDEQ(environmentID),
		).
		SetName(grp.Name).
		SetUpdatedBy(types.GetUserID(ctx)).
		Save(ctx)

	if err != nil {
		r.log.Error("Failed to update group", "error", err, "group_id", grp.ID)
		return ierr.WithError(err).
			WithHint("Failed to update group").
			Mark(ierr.ErrDatabase)
	}

	return nil
}

func (r *groupRepository) Delete(ctx context.Context, id string) error {
	client := r.client.Querier(ctx)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	// Soft delete by updating status
	_, err := client.Group.UpdateOneID(id).
		Where(
			group.TenantIDEQ(tenantID),
			group.EnvironmentIDEQ(environmentID),
		).
		SetStatus(string(types.StatusDeleted)).
		SetUpdatedBy(types.GetUserID(ctx)).
		Save(ctx)

	if err != nil {
		r.log.Error("Failed to delete group", "error", err, "group_id", id)
		return ierr.WithError(err).
			WithHint("Failed to delete group").
			Mark(ierr.ErrDatabase)
	}

	return nil
}

func (r *groupRepository) GetPricesInGroup(ctx context.Context, groupID string) ([]string, error) {
	client := r.client.Querier(ctx)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	entPrices, err := client.Price.Query().
		Where(
			price.GroupIDEQ(groupID),
			price.TenantIDEQ(tenantID),
			price.EnvironmentIDEQ(environmentID),
		).
		All(ctx)

	if err != nil {
		r.log.Error("Failed to get prices in group", "error", err, "group_id", groupID)
		return nil, ierr.WithError(err).
			WithHint("Failed to get prices in group").
			Mark(ierr.ErrDatabase)
	}

	priceIDs := make([]string, len(entPrices))
	for i, price := range entPrices {
		priceIDs[i] = price.ID
	}

	return priceIDs, nil
}

func (r *groupRepository) UpdatePriceGroup(ctx context.Context, priceID string, groupID *string) error {
	client := r.client.Querier(ctx)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	update := client.Price.UpdateOneID(priceID).
		Where(
			price.TenantIDEQ(tenantID),
			price.EnvironmentIDEQ(environmentID),
		)

	if groupID != nil {
		update = update.SetGroupID(*groupID)
	} else {
		update = update.ClearGroupID()
	}

	_, err := update.Save(ctx)
	if err != nil {
		r.log.Error("Failed to update price group", "error", err, "price_id", priceID, "group_id", groupID)
		return ierr.WithError(err).
			WithHint("Failed to update price group").
			Mark(ierr.ErrDatabase)
	}

	return nil
}

func (r *groupRepository) ValidatePricesExist(ctx context.Context, priceIDs []string) error {
	client := r.client.Querier(ctx)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	count, err := client.Price.Query().
		Where(
			price.IDIn(priceIDs...),
			price.TenantIDEQ(tenantID),
			price.EnvironmentIDEQ(environmentID),
		).
		Count(ctx)

	if err != nil {
		r.log.Error("Failed to validate prices exist", "error", err)
		return ierr.WithError(err).
			WithHint("Failed to validate prices exist").
			Mark(ierr.ErrDatabase)
	}

	if count != len(priceIDs) {
		return ierr.NewError("one or more prices not found").
			WithHint("One or more price IDs are invalid").
			Mark(ierr.ErrValidation)
	}

	return nil
}

func (r *groupRepository) ValidatePricesNotInOtherGroup(ctx context.Context, priceIDs []string, excludeGroupID string) error {
	client := r.client.Querier(ctx)
	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)

	var predicates []predicate.Price
	predicates = append(predicates,
		price.IDIn(priceIDs...),
		price.TenantIDEQ(tenantID),
		price.EnvironmentIDEQ(environmentID),
		price.GroupIDNotNil(),
	)

	if excludeGroupID != "" {
		predicates = append(predicates, price.GroupIDNEQ(excludeGroupID))
	}

	count, err := client.Price.Query().
		Where(predicates...).
		Count(ctx)

	if err != nil {
		r.log.Error("Failed to validate prices not in other group", "error", err)
		return ierr.WithError(err).
			WithHint("Failed to validate prices not in other group").
			Mark(ierr.ErrDatabase)
	}

	if count > 0 {
		return ierr.NewError("one or more prices are already in another group").
			WithHint("One or more prices are already assigned to another group").
			Mark(ierr.ErrValidation)
	}

	return nil
}

func (r *groupRepository) toDomainGroup(entGroup *ent.Group) *domainGroup.Group {
	return &domainGroup.Group{
		ID:            entGroup.ID,
		Name:          entGroup.Name,
		EntityType:    entGroup.EntityType,
		EnvironmentID: entGroup.EnvironmentID,
		BaseModel: types.BaseModel{
			TenantID:  entGroup.TenantID,
			Status:    types.Status(entGroup.Status),
			CreatedAt: entGroup.CreatedAt,
			UpdatedAt: entGroup.UpdatedAt,
			CreatedBy: entGroup.CreatedBy,
			UpdatedBy: entGroup.UpdatedBy,
		},
	}
}
