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

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// SubscriptionPlan holds the schema definition for the SubscriptionPlan entity.
//
// 删除策略：硬删除
// SubscriptionPlan 使用硬删除而非软删除，原因如下：
//   - 套餐为管理员维护的商品配置，删除即表示下架移除
//   - 通过 for_sale 字段控制是否在售，删除仅用于彻底移除
//   - 已购买的订阅记录保存在 UserSubscription 中，不受套餐删除影响
type SubscriptionPlan struct {
	ent.Schema
}

func (SubscriptionPlan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_plans"},
	}
}

func (SubscriptionPlan) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("group_id"),
		field.String("plan_type").
			MaxLen(40).
			Default(domain.SubscriptionPlanTypeSubscription),
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("description").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.Float("price").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Float("original_price").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Optional().
			Nillable(),
		field.Float("daily_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("weekly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("monthly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Int("concurrency_limit").
			Optional().
			Nillable(),
		field.String("currency").
			MaxLen(3).
			Default(""),
		field.Int("validity_days").
			Default(30),
		field.String("validity_unit").
			MaxLen(10).
			Default("day"),
		field.String("features").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.String("product_name").
			MaxLen(100).
			Default(""),
		field.Bool("for_sale").
			Default(true),
		field.Int("sort_order").
			Default(0),
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

func (SubscriptionPlan) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("groups", Group.Type).
			Through("group_bindings", SubscriptionPlanGroup.Type),
		edge.To("subscriptions", UserSubscription.Type),
	}
}

func (SubscriptionPlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id"),
		index.Fields("plan_type"),
		index.Fields("for_sale"),
	}
}
