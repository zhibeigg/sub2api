package repository

import (
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/stretchr/testify/require"
)

func TestAccountEffectivePlanTypePredicateBuildsValidPostgresShape(t *testing.T) {
	builder := entsql.Dialect(dialect.Postgres)
	table := builder.Table(dbaccount.Table)
	selector := builder.Select(table.C(dbaccount.FieldID)).From(table)
	accountEffectivePlanTypePredicate(" Plus ")(selector)

	query, args := selector.Query()
	require.Equal(t, []any{"openai", "plus", "plus"}, args)
	require.Contains(t, query, `#>> '{plan_type}'`)
	require.Contains(t, query, `EXISTS (SELECT`)
	require.NotContains(t, query, "?")
	require.Contains(t, query, `= $2`)
	require.Contains(t, query, `= $3`)
}
