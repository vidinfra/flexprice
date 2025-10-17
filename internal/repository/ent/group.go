package ent

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/group"
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
		SetEntityType(string(grp.EntityType)).
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
			group.StatusEQ(string(types.StatusPublished)),
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
			group.StatusEQ(string(types.StatusPublished)),
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
		SetStatus(string(types.StatusArchived)).
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

func (r *groupRepository) toDomainGroup(entGroup *ent.Group) *domainGroup.Group {
	return &domainGroup.Group{
		ID:            entGroup.ID,
		Name:          entGroup.Name,
		EntityType:    types.GroupEntityType(entGroup.EntityType),
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
