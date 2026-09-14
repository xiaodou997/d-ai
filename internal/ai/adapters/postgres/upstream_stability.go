package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/runtimecompat"
	"xiaodou/dai/internal/ai/serving"
)

// RecordAttempt persists completed provider calls before another route runs.
// Sealing repeats the same upsert, repairing transient write failures safely.
func (s *RequestStore) RecordAttempt(ctx context.Context, req *serving.Request, index int) error {
	if index < 0 || index >= len(req.Attempts) || req.RecordRegistrationRejected {
		return nil
	}
	var at time.Time
	if err := s.pool.QueryRow(ctx, `SELECT registered_at FROM ai_request_keys WHERE request_id=$1`, req.RequestID).Scan(&at); err != nil {
		return err
	}
	return insertUpstreamAttempt(ctx, s.pool, at, req.RequestID, index+1, req.Attempts[index])
}
func insertUpstreamAttempt(ctx context.Context, db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, at time.Time, id string, ordinal int, a serving.AttemptRecord) error {
	if ordinal > 0 && a.CandidateID != "" && !a.OutcomeFinal {
		return nil
	}
	a.ErrorMsg = serving.RedactInternalErrorDetail(a.ErrorMsg)
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	kind, resource := "direct_upstream", a.AccountID
	if a.PoolID != "" {
		kind, resource = "oauth_pool", a.PoolID
	}
	outcome := a.AvailabilityOutcome
	if outcome == "" {
		outcome = a.Outcome.String()
	}
	latency := a.TotalMs
	if a.Stream {
		latency = a.FirstOutputMs
	}
	completed := a.CompletedAt
	if completed.IsZero() {
		completed = time.Now()
	}
	_, err = db.Exec(ctx, `INSERT INTO ai_request_attempts(created_at,request_id,ordinal,facts,upstream_kind,upstream_resource_id,endpoint_id,credential_id,model_code,upstream_model,operation,stream,completed_at,outcome,cooldown_entered,latency_ms)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
 ON CONFLICT(created_at,request_id,ordinal) DO UPDATE SET facts=EXCLUDED.facts,outcome=EXCLUDED.outcome,cooldown_entered=EXCLUDED.cooldown_entered,latency_ms=EXCLUDED.latency_ms,completed_at=EXCLUDED.completed_at`,
		at, id, ordinal, raw, kind, resource, a.EndpointID, a.CredentialID, a.ModelCode, a.UpstreamModel, a.Operation, a.Stream, completed, outcome, a.CooldownEntered, latency)
	return err
}

func (s *RequestStore) ListUpstreamStability(ctx context.Context, kind, window string) ([]domain.UpstreamStability, error) {
	hours := 24
	switch window {
	case "1h":
		hours = 1
	case "7d":
		hours = 168
	}
	rows, err := s.pool.Query(ctx, `WITH resources AS (
 SELECT 'direct_upstream'::text kind,id::text id,status, (SELECT count(*) FROM ai_upstream_account_endpoints e WHERE e.account_id=a.id AND e.status='active') endpoints,0::bigint credentials FROM ai_upstream_accounts a
 UNION ALL SELECT 'oauth_pool',id::text,status,0::bigint,(SELECT count(*) FROM ai_provider_oauth_credentials c WHERE c.pool_id=p.id AND c.status='active') FROM ai_credential_pools p
 ), counts AS (
 SELECT upstream_kind,upstream_resource_id,
 COUNT(*) FILTER(WHERE completed_at>=now()-make_interval(hours=>$2) AND outcome='success') successes,
 COUNT(*) FILTER(WHERE completed_at>=now()-make_interval(hours=>$2) AND outcome IN ('server_error','timeout','network_error','unauthorized','rate_limited','model_error')) failures,
 COUNT(*) FILTER(WHERE completed_at>=now()-make_interval(hours=>$2) AND outcome NOT IN ('success','server_error','timeout','network_error','unauthorized','rate_limited','model_error')) excluded,
 COUNT(*) FILTER(WHERE completed_at>=now()-interval '1 hour' AND cooldown_entered) cooldowns
 FROM ai_request_attempts WHERE completed_at>=now()-make_interval(hours=>$2) AND upstream_kind IS NOT NULL
 GROUP BY upstream_kind,upstream_resource_id)
 SELECT r.kind,r.id,r.status,COALESCE(c.successes,0),COALESCE(c.failures,0),COALESCE(c.excluded,0),COALESCE(c.cooldowns,0),m.started_at,r.endpoints,r.credentials,
 (SELECT count(DISTINCT u.upstream_model_name) FROM ai_upstream_models u WHERE u.upstream_kind=r.kind AND u.upstream_id=r.id::uuid AND u.status='active')
 FROM resources r CROSS JOIN ai_upstream_runtime_metadata m LEFT JOIN counts c ON c.upstream_kind=r.kind AND c.upstream_resource_id=r.id
 WHERE ($1='' OR r.kind=$1) ORDER BY r.kind,r.id`, kind, hours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.UpstreamStability{}
	for rows.Next() {
		var x domain.UpstreamStability
		var started time.Time
		if err = rows.Scan(&x.ResourceKind, &x.ResourceID, &x.ConfigStatus, &x.Successes, &x.Failures, &x.Excluded, &x.CooldownsLastHour, &started, &x.EndpointCount, &x.CredentialCount, &x.ModelCount); err != nil {
			return nil, err
		}
		x.Window = window
		x.CoverageStartedAt = started.UnixMilli()
		x.Samples = x.Successes + x.Failures
		x.Models = []domain.UpstreamModelStability{}
		if x.Samples > 0 {
			rate := float64(x.Successes) * 100 / float64(x.Samples)
			x.SuccessRate = &rate
			x.StabilityDeclining = x.Samples >= 20 && rate < 95
		}
		x.RepeatedFailure = x.CooldownsLastHour >= 3
		out = append(out, x)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	paths, err := s.upstreamRuntimePaths(ctx, kind)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Paths = paths[out[i].ResourceKind+":"+out[i].ResourceID]
		if out[i].Paths == nil {
			out[i].Paths = []domain.UpstreamRuntimePath{}
		}
	}
	return out, nil
}
func (s *RequestStore) UpstreamStabilityDetails(ctx context.Context, kind, id, window string) ([]domain.UpstreamModelStability, error) {
	hours := 24
	if window == "1h" {
		hours = 1
	}
	if window == "7d" {
		hours = 168
	}
	rows, err := s.pool.Query(ctx, `SELECT COALESCE(endpoint_id,''),COALESCE(credential_id,''),COALESCE(upstream_model,''),COALESCE(operation,''),COALESCE(stream,false),outcome,count(*),
 COALESCE(percentile_cont(0.5) WITHIN GROUP(ORDER BY latency_ms) FILTER(WHERE outcome='success' AND latency_ms>0),0),
 COALESCE(percentile_cont(0.95) WITHIN GROUP(ORDER BY latency_ms) FILTER(WHERE outcome='success' AND latency_ms>0),0),
 COALESCE((array_agg(facts->>'ErrorMsg' ORDER BY completed_at DESC) FILTER(WHERE outcome<>'success'))[1],'')
 FROM ai_request_attempts WHERE upstream_kind=$1 AND upstream_resource_id=$2 AND completed_at>=now()-make_interval(hours=>$3)
 GROUP BY endpoint_id,credential_id,upstream_model,operation,stream,outcome ORDER BY upstream_model,endpoint_id,outcome`, kind, id, hours)
	if err != nil {
		return nil, fmt.Errorf("upstream statistics: %w", err)
	}
	defer rows.Close()
	out := []domain.UpstreamModelStability{}
	for rows.Next() {
		var x domain.UpstreamModelStability
		if err = rows.Scan(&x.EndpointID, &x.CredentialID, &x.Model, &x.Operation, &x.Stream, &x.Outcome, &x.Count, &x.P50Ms, &x.P95Ms, &x.LastError); err != nil {
			return nil, err
		}
		x.LastError = serving.RedactInternalErrorDetail(x.LastError)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *RequestStore) ResumeUpstreamCredentials(ctx context.Context, kind, id string) error {
	if kind != "oauth_pool" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `UPDATE ai_provider_oauth_credentials SET cooldown_until=NULL WHERE pool_id=$1::uuid AND status='active'`, id)
	return err
}

// Enumerate configured physical paths to aggregate availability without treating
// an unrelated healthy parent or a deleted endpoint as a working model path.
func (s *RequestStore) upstreamRuntimePaths(ctx context.Context, kind string) (map[string][]domain.UpstreamRuntimePath, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.upstream_kind,u.upstream_id::text,e.id::text,''::text,u.upstream_model_name,u.capability_type,e.api_format,''::text
 FROM ai_upstream_models u JOIN ai_upstream_accounts a ON a.id=u.upstream_id JOIN ai_upstream_account_endpoints e ON e.account_id=a.id
 WHERE u.upstream_kind='direct_upstream' AND u.status='active' AND e.status='active' AND btrim(a.api_key_ciphertext)<>'' AND ($1='' OR $1='direct_upstream')
 UNION ALL SELECT u.upstream_kind,u.upstream_id::text,'',c.id::text,u.upstream_model_name,u.capability_type,'',p.fixed_provider_type
 FROM ai_upstream_models u JOIN ai_credential_pools p ON p.id=u.upstream_id JOIN ai_provider_oauth_credentials c ON c.pool_id=p.id
 WHERE u.upstream_kind='oauth_pool' AND u.status='active' AND c.status='active' AND ($1='' OR $1='oauth_pool')`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]domain.UpstreamRuntimePath{}
	for rows.Next() {
		var kind, id, capability, protocol, provider string
		var path domain.UpstreamRuntimePath
		if err = rows.Scan(&kind, &id, &path.EndpointID, &path.CredentialID, &path.Model, &capability, &protocol, &provider); err != nil {
			return nil, err
		}
		p := domain.UpstreamProtocol(protocol)
		if kind == "oauth_pool" {
			p = domain.FixedProviderProtocol(domain.FixedProviderType(provider))
		}
		cap := domain.CapabilityType(capability)
		// Use the same protocol/capability families as the execution binding.
		switch cap {
		case domain.CapabilityChat:
			if p != domain.ProtocolOpenAIChat && p != domain.ProtocolOpenAIResponses && p != domain.ProtocolAnthropicMessages && p != domain.ProtocolGeminiGenerate {
				continue
			}
		case domain.CapabilityImage:
			if p != domain.ProtocolOpenAIImages && p != domain.ProtocolGeminiGenerate {
				continue
			}
		case domain.CapabilityEmbedding:
			if p != domain.ProtocolOpenAIEmbeddings && p != domain.ProtocolGeminiEmbeddings {
				continue
			}
		}
		op, e := runtimecompat.ProtocolToSurfaceForCapability(p, runtimecompat.CapabilityToCore(cap))
		if e != nil {
			continue
		}
		path.Operation = string(op)
		key := kind + ":" + id
		out[key] = append(out[key], path)
		if p == domain.ProtocolOpenAIResponses {
			path.Operation = string(op) + ":compact"
			out[key] = append(out[key], path)
		}
		if cap == domain.CapabilityImage {
			path.Operation = string(op) + ":edit"
			out[key] = append(out[key], path)
		}
	}
	return out, rows.Err()
}
