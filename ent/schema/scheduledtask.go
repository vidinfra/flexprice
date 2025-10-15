package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	baseMixin "github.com/flexprice/flexprice/ent/schema/mixin"
)

// ScheduledTask holds the schema definition for the ScheduledTask entity.
// This entity manages scheduled export tasks for various entity types to external destinations (S3, etc.)
type ScheduledTask struct {
	ent.Schema
}

// Mixin of the ScheduledTask.
func (ScheduledTask) Mixin() []ent.Mixin {
	return []ent.Mixin{
		baseMixin.BaseMixin{},
		baseMixin.EnvironmentMixin{},
	}
}

// Fields of the ScheduledTask.
func (ScheduledTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			Unique().
			Immutable(),
		field.String("connection_id").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Comment("Reference to the connection (S3, etc.) to use for export"),
		field.String("entity_type").
			SchemaType(map[string]string{
				"postgres": "varchar(50)",
			}).
			NotEmpty().
			Comment("Entity type to export (feature_usage, customer, invoice, etc.)"),
		field.String("interval").
			SchemaType(map[string]string{
				"postgres": "varchar(20)",
			}).
			NotEmpty().
			Comment("Schedule interval (hourly, daily, weekly, monthly)"),
		field.Bool("enabled").
			Default(true).
			Comment("Whether this scheduled job is active"),
		field.JSON("job_config", map[string]interface{}{}).
			Optional().
			Comment("Job-specific configuration (bucket, region, key_prefix, compression, etc.)"),
		field.Time("last_run_at").
			Optional().
			Nillable().
			Comment("Timestamp of the last successful run"),
		field.Time("next_run_at").
			Optional().
			Nillable().
			Comment("Timestamp for the next scheduled run"),
		field.String("last_run_status").
			SchemaType(map[string]string{
				"postgres": "varchar(20)",
			}).
			Optional().
			Comment("Status of the last run (success, failed, running)"),
		field.Text("last_run_error").
			Optional().
			Comment("Error message from last run if failed"),
		field.String("temporal_schedule_id").
			SchemaType(map[string]string{
				"postgres": "varchar(100)",
			}).
			Optional().
			Comment("Temporal schedule ID for the recurring workflow"),
	}
}

// Indexes of the ScheduledTask.
func (ScheduledTask) Indexes() []ent.Index {
	return []ent.Index{
		// Index for finding jobs by tenant and environment
		index.Fields("tenant_id", "environment_id", "enabled"),
		// Index for finding jobs by connection
		index.Fields("connection_id", "enabled"),
		// Index for finding jobs by entity type and interval
		index.Fields("entity_type", "interval", "enabled"),
		// Index for finding jobs that need to run
		index.Fields("enabled", "next_run_at"),
		// Unique constraint: one job per connection + entity_type
		index.Fields("connection_id", "entity_type").
			Unique(),
	}
}
