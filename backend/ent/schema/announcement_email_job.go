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

type AnnouncementEmailJob struct{ ent.Schema }

func (AnnouncementEmailJob) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "announcement_email_jobs"}}
}

func (AnnouncementEmailJob) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("announcement_id"),
		field.String("status").MaxLen(32).Default("pending"),
		field.Time("scheduled_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("recipient_cutoff_id").Default(0),
		field.Int64("preparation_cursor_id").Default(0),
		field.Int64("recipient_count").Default(0),
		field.Int64("pending_count").Default(0),
		field.Int64("sending_count").Default(0),
		field.Int64("sent_count").Default(0),
		field.Int64("failed_count").Default(0),
		field.Int64("ambiguous_count").Default(0),
		field.Int64("skipped_count").Default(0),
		field.Int("attempt_count").Default(0),
		field.Int64("created_by").Optional().Nillable(),
		field.String("last_error_code").Optional().Nillable().MaxLen(128),
		field.String("announcement_title").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("announcement_content").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("announcement_starts_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("lease_owner").Optional().Nillable().MaxLen(128),
		field.Time("lease_expires_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("last_error").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("started_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("finished_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (AnnouncementEmailJob) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("announcement", Announcement.Type).Ref("email_jobs").Field("announcement_id").Required().Unique(),
		edge.To("deliveries", AnnouncementEmailDelivery.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (AnnouncementEmailJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("announcement_id").Unique(),
		index.Fields("status", "scheduled_at"),
		index.Fields("lease_expires_at"),
	}
}
