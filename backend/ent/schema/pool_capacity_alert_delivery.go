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

// PoolCapacityAlertDelivery stores per-recipient, per-channel retry state.
type PoolCapacityAlertDelivery struct{ ent.Schema }

func (PoolCapacityAlertDelivery) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "pool_capacity_alert_deliveries"}}
}

func (PoolCapacityAlertDelivery) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("event_id"),
		field.String("channel").MaxLen(24),
		field.Int64("recipient_user_id"),
		field.Int64("identity_channel_id").Default(0),
		field.String("recipient_email").MaxLen(255).Default(""),
		field.String("recipient_name").MaxLen(100).Default(""),
		field.String("locale").MaxLen(16).Default("en"),
		field.String("status").MaxLen(24).Default("pending"),
		field.Int("attempt_count").Default(0),
		field.Int("max_attempts").Default(6),
		field.Time("next_attempt_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("lease_owner").Optional().Nillable().MaxLen(128),
		field.Time("lease_expires_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("last_error_class").Optional().Nillable().MaxLen(32),
		field.String("last_error").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("sent_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PoolCapacityAlertDelivery) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("event_id", "channel", "recipient_user_id", "identity_channel_id").Unique(),
		index.Fields("status", "next_attempt_at"),
		index.Fields("lease_expires_at"),
	}
}
