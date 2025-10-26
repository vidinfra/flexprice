package types

import ierr "github.com/flexprice/flexprice/internal/errors"

// CostsheetV2Filter represents the filter options for costsheet v2
type CostsheetV2Filter struct {
	*QueryFilter
	*TimeRangeFilter

	// filters allows complex filtering based on multiple fields
	Filters        []*FilterCondition `json:"filters,omitempty" form:"filters" validate:"omitempty"`
	Sort           []*SortCondition   `json:"sort,omitempty" form:"sort" validate:"omitempty"`
	CostsheetV2IDs []string           `json:"costsheet_v2_ids,omitempty" form:"costsheet_v2_ids" validate:"omitempty"`
}

// NewCostsheetV2Filter creates a new costsheet v2 filter with default options
func NewCostsheetV2Filter() *CostsheetV2Filter {
	return &CostsheetV2Filter{
		QueryFilter: NewDefaultQueryFilter(),
	}
}

// NewNoLimitCostsheetV2Filter creates a new costsheet v2 filter without pagination
func NewNoLimitCostsheetV2Filter() *CostsheetV2Filter {
	return &CostsheetV2Filter{
		QueryFilter: NewNoLimitQueryFilter(),
	}
}

// Validate validates the filter options
func (f *CostsheetV2Filter) Validate() error {
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

	for _, costsheetID := range f.CostsheetV2IDs {
		if costsheetID == "" {
			return ierr.NewError("costsheet v2 id can not be empty").
				WithHint("Costsheet v2 id can not be empty").
				Mark(ierr.ErrValidation)
		}
	}
	return nil
}

// GetLimit implements BaseFilter interface
func (f *CostsheetV2Filter) GetLimit() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetLimit()
	}
	return f.QueryFilter.GetLimit()
}

// GetOffset implements BaseFilter interface
func (f *CostsheetV2Filter) GetOffset() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOffset()
	}
	return f.QueryFilter.GetOffset()
}

// GetStatus implements BaseFilter interface
func (f *CostsheetV2Filter) GetStatus() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetStatus()
	}
	return f.QueryFilter.GetStatus()
}

// GetSort implements BaseFilter interface
func (f *CostsheetV2Filter) GetSort() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetSort()
	}
	return f.QueryFilter.GetSort()
}

// GetOrder implements BaseFilter interface
func (f *CostsheetV2Filter) GetOrder() string {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOrder()
	}
	return f.QueryFilter.GetOrder()
}

// GetExpand implements BaseFilter interface
func (f *CostsheetV2Filter) GetExpand() Expand {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetExpand()
	}
	return f.QueryFilter.GetExpand()
}

func (f *CostsheetV2Filter) IsUnlimited() bool {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().IsUnlimited()
	}
	return f.QueryFilter.IsUnlimited()
}
