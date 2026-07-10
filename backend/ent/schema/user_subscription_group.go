package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserSubscriptionGroup binds a shared-quota subscription to an allowed group.
type UserSubscriptionGroup struct {
	ent.Schema
}

func (UserSubscriptionGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_subscription_groups"},
		field.ID("subscription_id", "group_id"),
	}
}

func (UserSubscriptionGroup) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("subscription_id"),
		field.Int64("user_id"),
		field.Int64("group_id"),
		field.Bool("enabled").Default(true),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UserSubscriptionGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("subscription", UserSubscription.Type).
			Unique().
			Required().
			Field("subscription_id"),
		edge.To("group", Group.Type).
			Unique().
			Required().
			Field("group_id"),
	}
}

func (UserSubscriptionGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id"),
		index.Fields("user_id", "group_id").
			Unique().
			Annotations(entsql.IndexWhere("enabled = true")),
		index.Fields("subscription_id", "enabled"),
	}
}
