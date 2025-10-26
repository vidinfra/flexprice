package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	baseMixin "github.com/flexprice/flexprice/ent/schema/mixin"
)

// CostsheetV2 represents the version 2 of the costsheet entity for tracking cost-related configurations.
// This entity includes basic columns as specified in the requirements:
// - id, name, tenant_id, environment_id, status, created_at, created_by, updated_at, updated_by
// This is used for comparing revenue and costsheet calculations.
type CostsheetV2 struct {
	ent.Schema
}

// Mixin injects common fields into the schema.
// We include:
// 1. BaseMixin: Common fields for all entities
//   - tenant_id: Multi-tenancy support
//   - status: Record status (published, draft, etc.)
//   - created_at, updated_at: Timestamps
//   - created_by, updated_by: Audit fields
//
// 2. EnvironmentMixin: Environment segregation
//   - environment_id: Separates data by environment (prod, staging, etc.)
func (CostsheetV2) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},        // tenant_id, status, created_at, updated_at, created_by, updated_by
		baseMixin.EnvironmentMixin{}, // environment_id
		baseMixin.MetadataMixin{},
	}
}

// Schema fields for the CostsheetV2 entity.
func (CostsheetV2) Fields() []ent.Field {
	return []ent.Field{
		field.String("id"). // Primary key
					SchemaType(map[string]string{
				"postgres": "varchar(50)", // Consistent with other entity IDs
			}).
			Unique().    // Ensures uniqueness across all records
			Immutable(), // ID cannot be changed once set

		field.String("name"). // Name of the costsheet
					SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			NotEmpty(), // Must have a name

		field.String("lookup_key"). // Lookup key for easy identification
						SchemaType(map[string]string{
				"postgres": "varchar(255)",
			}).
			Optional(),

		field.Text("description"). // Description of the costsheet
						Optional(),
	}
}

// Edges defines relationships with other entities.
// CostsheetV2 can have multiple prices associated with it.
func (CostsheetV2) Edges() []ent.Edge {
	return []ent.Edge{
		// Prices edge: A costsheet v2 can have many prices
		edge.From("prices", Price.Type).
			Ref("costsheet_v2"),
	}
}

// Indexes improves query performance and ensures data integrity.
// We index:
// 1. tenant_id + environment_id: For fast multi-tenant, multi-environment queries
// 2. tenant_id + environment_id + lookup_key: For unique lookup key queries
func (CostsheetV2) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "environment_id", "lookup_key").
			Unique().
			StorageKey(Idx_tenant_environment_lookup_key).
			Annotations(entsql.IndexWhere("status = 'published'" + " AND lookup_key IS NOT NULL AND lookup_key != ''")),
		index.Fields("tenant_id", "environment_id"), // Supports multi-tenant, multi-environment queries
	}
}
