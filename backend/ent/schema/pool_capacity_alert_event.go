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

// PoolCapacityAlertEvent is an immutable low-capacity alert episode.
type PoolCapacityAlertEvent struct{ ent.Schema }

func (PoolCapacityAlertEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "pool_capacity_alert_events"}}
}

func (PoolCapacityAlertEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("state_id"),
		field.Int64("episode"),
		field.Int64("group_id"),
		field.Int64("group_generation").Default(0),
		field.Int64("account_id"),
		field.Int64("api_key_id"),
		field.Int64("user_id"),
		field.Int8("billing_type"),
		field.String("group_name").MaxLen(255).Default(""),
		field.String("account_name").MaxLen(255).Default(""),
		field.String("api_key_name").MaxLen(255).Default(""),
		field.String("user_email").MaxLen(255).Default(""),
		field.Int64("predicted_requests"),
		field.Int64("threshold_requests").Default(50),
		field.Int64("account_requests").Optional().Nillable(),
		field.Int64("api_key_requests").Optional().Nillable(),
		field.Int64("wallet_requests").Optional().Nillable(),
		field.Float("avg_account_cost").SchemaType(map[string]string{dialect.Postgres: "numeric(30,12)"}),
		field.Float("avg_actual_cost").SchemaType(map[string]string{dialect.Postgres: "numeric(30,12)"}),
		field.Float("account_remaining").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(30,12)"}),
		field.Float("api_key_remaining").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(30,12)"}),
		field.Float("wallet_remaining").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(30,12)"}),
		field.Int("sample_count").Default(50),
		field.String("bottleneck").MaxLen(32).Default(""),
		field.String("qqbot_app_id").MaxLen(128).Default(""),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PoolCapacityAlertEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("state_id", "episode").Unique(),
		index.Fields("group_id", "created_at"),
		index.Fields("created_at"),
	}
}
