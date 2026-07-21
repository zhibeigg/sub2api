package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PoolCapacityAlertState stores the current prediction state for one final
// group/account/API-key/user billing context.
type PoolCapacityAlertState struct{ ent.Schema }

func (PoolCapacityAlertState) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "pool_capacity_alert_states"}}
}

func (PoolCapacityAlertState) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("group_id"),
		field.Int64("group_generation").Default(0),
		field.Int64("account_id"),
		field.Int64("api_key_id"),
		field.Int64("user_id"),
		field.Int8("billing_type"),
		field.String("status").MaxLen(16).Default("healthy"),
		field.Int64("episode").Default(0),
		field.Int64("predicted_requests").Optional().Nillable(),
		field.Int64("account_requests").Optional().Nillable(),
		field.Int64("api_key_requests").Optional().Nillable(),
		field.Int64("wallet_requests").Optional().Nillable(),
		field.Float("avg_account_cost").Default(0).SchemaType(map[string]string{dialect.Postgres: "numeric(30,12)"}),
		field.Float("avg_actual_cost").Default(0).SchemaType(map[string]string{dialect.Postgres: "numeric(30,12)"}),
		field.Int("sample_count").Default(0),
		field.String("bottleneck").MaxLen(32).Default(""),
		field.Time("last_evaluated_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_alerted_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PoolCapacityAlertState) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id", "group_generation", "account_id", "api_key_id", "user_id", "billing_type").Unique(),
		index.Fields("group_id", "group_generation"),
		index.Fields("status", "updated_at"),
	}
}
