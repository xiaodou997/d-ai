package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"
	commercial "xiaodou/dai/internal/ai/commercial"
	coreruntime "xiaodou/dai/internal/ai/core/runtime"
	dbgen "xiaodou/dai/internal/ai/db/gen"
)

const (
	legacyRoutingScopeGlobal = "global"
)

// CommercialRepo adapts the current group/limit/route-weight storage to the
// rebuilt commercial repository port. This bridge is intentionally honest:
// only capabilities that already have a real legacy backing store are wired.
// Features that need the rebuilt schema stay explicitly unavailable instead of
// being silently faked.
type CommercialRepo struct {
	q               *dbgen.Queries
	pool            *translatingPool
	group           *GroupRepo
	limit           *LimitRepo
	weights         *RouteWeightsStore
	runtimeResolver *coreruntime.Resolver
}

// WithRuntimeResolver makes dispatch preview use the same planner and binder
// as live requests. It is set during server wiring after the repository exists.
func (r *CommercialRepo) WithRuntimeResolver(resolver *coreruntime.Resolver) *CommercialRepo {
	r.runtimeResolver = resolver
	return r
}

func NewCommercialRepo(q *dbgen.Queries, pool *pgxpool.Pool) *CommercialRepo {
	return &CommercialRepo{
		q:       q,
		pool:    newTranslatingPool(pool),
		group:   NewGroupRepo(q, pool),
		limit:   NewLimitRepo(q),
		weights: NewRouteWeightsStore(pool),
	}
}

var _ commercial.Repository = (*CommercialRepo)(nil)
