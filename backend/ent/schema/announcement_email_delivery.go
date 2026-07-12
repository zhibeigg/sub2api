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

type AnnouncementEmailDelivery struct{ ent.Schema }

func (AnnouncementEmailDelivery) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "announcement_email_deliveries"}}
}

func (AnnouncementEmailDelivery) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("job_id"),
		field.Int64("user_id").Immutable(),
		field.String("recipient_email").MaxLen(255).Immutable(),
		field.String("recipient_name").MaxLen(100).Default("").Immutable(),
		field.String("locale").MaxLen(16).Default("en"),
		field.String("status").MaxLen(32).Default("pending"),
		field.Int("attempt_count").Default(0),
		field.Int("max_attempts").Default(5),
		field.Time("next_attempt_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("lease_owner").Optional().Nillable().MaxLen(128),
		field.Time("lease_expires_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("last_error_class").Optional().Nillable().MaxLen(32),
		field.String("last_error").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("sent_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (AnnouncementEmailDelivery) Edges() []ent.Edge {
	return []ent.Edge{edge.From("job", AnnouncementEmailJob.Type).Ref("deliveries").Field("job_id").Required().Unique()}
}

func (AnnouncementEmailDelivery) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("job_id", "user_id").Unique(),
		index.Fields("job_id", "status", "next_attempt_at"),
		index.Fields("status", "next_attempt_at"),
		index.Fields("lease_expires_at"),
	}
}
