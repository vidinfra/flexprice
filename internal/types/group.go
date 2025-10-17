package types

// GroupFilter represents the filter options for groups
type GroupFilter struct {
	*QueryFilter
	*TimeRangeFilter

	// filters allows complex filtering based on multiple fields
	Filters []*FilterCondition `json:"filters,omitempty" form:"filters" validate:"omitempty"`
	Sort    []*SortCondition   `json:"sort,omitempty" form:"sort" validate:"omitempty"`

	// Group specific filters
	GroupIDs   []string `form:"group_ids" json:"group_ids"`
	EntityType string   `form:"entity_type" json:"entity_type"`
	Name       string   `form:"name" json:"name"`
}

// NewGroupFilter creates a new group filter with default options
func NewGroupFilter() *GroupFilter {
	return &GroupFilter{
		QueryFilter: NewDefaultQueryFilter(),
	}
}

// NewNoLimitGroupFilter creates a new group filter without pagination
func NewNoLimitGroupFilter() *GroupFilter {
	return &GroupFilter{
		QueryFilter: NewNoLimitQueryFilter(),
	}
}

// Validate validates the group filter
func (f *GroupFilter) Validate() error {
	if f.QueryFilter != nil {
		if err := f.QueryFilter.Validate(); err != nil {
			return err
		}
	}

	if f.TimeRangeFilter != nil {
		if err := f.TimeRangeFilter.Validate(); err != nil {
			return err
		}
	}

	return nil
}
