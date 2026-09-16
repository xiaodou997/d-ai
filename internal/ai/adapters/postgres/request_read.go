package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

type recordCursor struct {
	At   time.Time `json:"at"`
	ID   string    `json:"id"`
	Kind string    `json:"kind"`
}

func (s *RequestStore) Records(ctx context.Context, q domain.RecordQuery) (domain.RecordPage, error) {
	return s.readRecords(ctx, q, "")
}
func (s *RequestStore) Record(ctx context.Context, scope domain.RecordScope, id string) (domain.RequestRecord, error) {
	page, err := s.readRecords(ctx, domain.RecordQuery{RecordScope: scope, Kind: "settlements", Limit: 1}, id)
	if err != nil {
		return domain.RequestRecord{}, err
	}
	if len(page.Records) == 0 {
		return domain.RequestRecord{}, domain.ErrNotFound
	}
	r := page.Records[0]
	if scope.Admin {
		rows, e := s.pool.Query(ctx, `SELECT a.facts || jsonb_build_object('ordinal',a.ordinal,'outcome',a.outcome,'account_name',COALESCE(u.name,'')) FROM ai_request_attempts a LEFT JOIN ai_upstream_accounts u ON u.id::text=a.facts->>'AccountID' WHERE a.request_id=$1 ORDER BY a.ordinal`, id)
		if e != nil {
			return r, e
		}
		defer rows.Close()
		for rows.Next() {
			var b []byte
			if e = rows.Scan(&b); e != nil {
				return r, e
			}
			r.Attempts = append(r.Attempts, json.RawMessage(b))
		}
		if e = rows.Err(); e != nil {
			return r, e
		}
	}
	return r, nil
}

// Shared by the record list and totals; name lookups always correlate to the
// settlement tenant so they cannot widen the authenticated record scope.
func recordSearchSQL(first int) string {
	return fmt.Sprintf(`
 AND ($%[1]d='' OR EXISTS (SELECT 1 FROM iam_tenants t WHERE t.tenant_id=s.tenant_id AND strpos(lower(t.tenant_name),lower($%[1]d))>0))
 AND ($%[2]d='' OR EXISTS (SELECT 1 FROM iam_accounts u WHERE u.tenant_id=s.tenant_id AND u.user_id=s.user_id AND (strpos(lower(u.username),lower($%[2]d))>0 OR strpos(lower(COALESCE(u.nickname,'')),lower($%[2]d))>0)))
 AND ($%[3]d='' OR s.dimensions->>'group_id'=$%[3]d OR strpos(lower(COALESCE(s.dimensions->>'group_name_snapshot','')),lower($%[3]d))>0 OR EXISTS (SELECT 1 FROM ai_groups g WHERE g.tenant_id=s.tenant_id AND g.id::text=s.dimensions->>'group_id' AND strpos(lower(g.name),lower($%[3]d))>0))
 AND ($%[4]d='' OR EXISTS (SELECT 1 FROM ai_api_keys k WHERE k.tenant_id=s.tenant_id AND k.id::text=s.api_key_id AND strpos(lower(k.name),lower($%[4]d))>0))
 AND ($%[5]d='' OR s.request_id=$%[5]d)
 `, first, first+1, first+2, first+3, first+4)
}

func (s *RequestStore) readRecords(ctx context.Context, q domain.RecordQuery, id string) (domain.RecordPage, error) {
	out := domain.RecordPage{Records: []domain.RequestRecord{}}
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	var cursorAt *time.Time
	cursorID := ""
	if q.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(q.Cursor)
		if e != nil {
			return out, domain.ErrInvalidRecordCursor
		}
		var c recordCursor
		if json.Unmarshal(raw, &c) != nil || c.ID == "" || c.At.IsZero() || c.Kind != q.Kind {
			return out, domain.ErrInvalidRecordCursor
		}
		cursorAt = &c.At
		cursorID = c.ID
	}
	// The final financial evidence is the durable anchor, including waived
	// decisions. Request and error partitions may already have expired.
	rows, err := s.pool.Query(ctx, `SELECT s.request_id,s.created_at,s.tenant_id,s.user_id,s.dimensions,s.evidence,s.pricing,
 s.state,s.reason,s.billing_source,s.tenant_due,s.user_due,s.tenant_charged,s.user_charged,s.subscription_due,s.key_due,s.posted_at,s.refunded_at,
 COALESCE(r.end_reason,'details_expired'),COALESCE(r.delivery_state,'unknown'),COALESCE(r.is_error,s.reason IN ('request_error_waived','unconfirmed_execution')),
 r.request_id IS NOT NULL,e.request_id IS NOT NULL,
 CASE WHEN e.request_id IS NULL THEN NULL ELSE jsonb_build_object('origin',e.origin,'stage',e.stage,'code',e.code,'message',e.message) END,COALESCE(e.internal_detail,''),s.last_error,
 COALESCE(t.tenant_name,''),COALESCE(u.username,''),s.api_key_id,COALESCE(k.name,''),COALESCE(k.last_four,''),COALESCE(g.name,''),COALESCE(a.name,''),s.refund_reason,s.refund_operator
 FROM bill_settlements s LEFT JOIN ai_requests r ON r.created_at=s.created_at AND r.request_id=s.request_id
 LEFT JOIN ai_request_errors e ON e.created_at=s.created_at AND e.request_id=s.request_id
 LEFT JOIN iam_tenants t ON t.tenant_id=s.tenant_id
 LEFT JOIN iam_accounts u ON u.tenant_id=s.tenant_id AND u.user_id=s.user_id
 LEFT JOIN ai_api_keys k ON k.tenant_id=s.tenant_id AND k.id::text=s.api_key_id
 LEFT JOIN ai_groups g ON g.tenant_id=s.tenant_id AND g.id::text=s.dimensions->>'group_id'
 LEFT JOIN ai_upstream_accounts a ON a.id::text=s.dimensions->>'upstream_account_id'
 WHERE ($1='' OR s.tenant_id=$1) AND ($2='' OR s.user_id=$2)
 AND ($3='' OR s.dimensions->>'model_code'=$3) AND ($4='' OR s.dimensions->>'request_source'=$4)
 AND ($5::timestamptz IS NULL OR s.created_at>=$5) AND ($6::timestamptz IS NULL OR s.created_at<$6)
 AND ($7::timestamptz IS NULL OR (s.created_at,s.request_id)<($7,$8))
 AND ($9='settlements' OR ($9='errors' AND e.request_id IS NOT NULL) OR ($9='requests' AND r.request_id IS NOT NULL AND NOT r.is_error))
 AND ($10='' OR s.request_id=$10)
 `+recordSearchSQL(12)+` ORDER BY s.created_at DESC,s.request_id DESC LIMIT $11`, q.TenantID, q.UserID, q.Model, q.Source, q.From, q.To, cursorAt, cursorID, q.Kind, id, q.Limit+1, q.TenantName, q.UserName, q.Group, q.APIKeyName, q.RequestID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.RequestRecord
		var dimensions, evidence, pricing, errorJSON []byte
		var tenantDue, tenantCharged int64
		var processingDetail, tenantName, userName, keyID, keyName, keyLastFour, groupName, accountName string
		if err = rows.Scan(&r.RequestID, &r.CreatedAt, &r.TenantID, &r.UserID, &dimensions, &evidence, &pricing, &r.Charge.State, &r.Charge.Reason, &r.Charge.Source, &tenantDue, &r.Charge.UserDue, &tenantCharged, &r.Charge.UserCharged, &r.Charge.SubscriptionUsed, &r.Charge.APIKeyUsed, &r.Charge.PostedAt, &r.Charge.RefundedAt, &r.EndReason, &r.Delivery, &r.IsError, &r.ExecutionAvailable, &r.DiagnosticsAvailable, &errorJSON, &r.InternalDetail, &processingDetail, &tenantName, &userName, &keyID, &keyName, &keyLastFour, &groupName, &accountName, &r.Charge.RefundReason, &r.Charge.RefundOperator); err != nil {
			return out, err
		}
		var facts struct {
			Model  string `json:"model_code"`
			Source string `json:"request_source"`
			Group  string `json:"group_id"`
			HTTP   *int   `json:"http_status"`
			Stream bool   `json:"stream"`
			First  *int   `json:"first_token_latency_ms"`
			Total  *int   `json:"request_total_ms"`
		}
		if err = json.Unmarshal(dimensions, &facts); err != nil {
			return out, err
		}
		if err = json.Unmarshal(dimensions, &r.Profile); err != nil {
			return out, err
		}
		r.Profile.APIKeyID, r.Profile.APIKeyName, r.Profile.APIKeyLastFour = keyID, keyName, keyLastFour
		if r.Profile.GroupName == "" {
			r.Profile.GroupName = groupName
		}
		if !q.EndUser {
			r.Profile.UserName = userName
		}
		if q.Admin {
			r.Profile.TenantName = tenantName
			r.AdminContext = &domain.RecordAdminContext{}
			if err = json.Unmarshal(dimensions, r.AdminContext); err != nil {
				return out, err
			}
			var frozen struct {
				TenantMultiplier *float64 `json:"tenant_multiplier"`
			}
			if err = json.Unmarshal(pricing, &frozen); err != nil {
				return out, err
			}
			r.AdminContext.TenantMultiplier = frozen.TenantMultiplier
			r.AdminContext.AccountName = accountName
		} else {
			r.Charge.RefundOperator = ""
		}
		if id == "" {
			r.Charge.RefundReason = ""
			r.Charge.RefundOperator = ""
		}
		r.Model = facts.Model
		r.Source = facts.Source
		r.HTTPStatus = facts.HTTP
		r.Stream = facts.Stream
		r.FirstTokenMs = facts.First
		r.TotalMs = facts.Total
		var proof struct {
			Usage          domain.UsageEvidence `json:"usage"`
			Reference      int64                `json:"reference_cost_micro"`
			MediaConfirmed bool                 `json:"media_confirmed"`
			MediaUnits     float64              `json:"media_units"`
			MediaType      string               `json:"media_unit_type"`
		}
		if err = json.Unmarshal(evidence, &proof); err != nil {
			return out, err
		}
		ptr := func(key string) *int64 {
			n, ok := proof.Usage.Fields[key]
			if !ok {
				return nil
			}
			v := int64(n)
			return &v
		}
		if proof.MediaConfirmed {
			r.MediaUnits = &proof.MediaUnits
			r.MediaUnitType = proof.MediaType
		}
		r.Tokens = domain.RecordTokens{Input: ptr("input_tokens"), Output: ptr("output_tokens"), CacheRead: ptr("cache_read_tokens"), CacheWrite: ptr("cache_write_tokens"), Reasoning: ptr("reasoning_tokens")}
		r.Tokens.Protocol = string(proof.Usage.Protocol)
		if proof.Usage.Reported() {
			normalized := proof.Usage.Tokens()
			total := int64(normalized.PromptTokens) + int64(normalized.CompletionTokens)
			r.Tokens.Total = &total
		}
		if len(errorJSON) > 0 {
			if err = json.Unmarshal(errorJSON, &r.Error); err != nil {
				return out, err
			}
		}
		if !q.EndUser {
			r.Charge.TenantDue = &tenantDue
			r.Charge.TenantCharged = &tenantCharged
		}
		if q.Admin {
			r.Charge.ReferenceCost = &proof.Reference
			if id != "" {
				r.Charge.ProcessingDetail = serving.RedactInternalErrorDetail(processingDetail)
			}
			if id != "" {
				r.Evidence = evidence
				r.Pricing = pricing
			}
		} else {
			r.InternalDetail = ""
			if id != "" {
				r.Evidence, _ = json.Marshal(map[string]any{"usage": proof.Usage, "media_units": r.MediaUnits, "media_unit_type": r.MediaUnitType})
				var frozen struct {
					Snapshot                domain.BillingSnapshot `json:"snapshot"`
					TenantMultiplier        float64                `json:"tenant_multiplier"`
					SubscriptionMultipliers map[string]float64     `json:"subscription_group_multipliers"`
					Calculation             struct {
						UserPayable json.RawMessage `json:"user_payable"`
					} `json:"calculation"`
				}
				if err = json.Unmarshal(pricing, &frozen); err != nil {
					return out, err
				}
				receipt := map[string]any{"retail_price": frozen.Snapshot.RetailEntry, "user_multiplier": frozen.Snapshot.EffectiveUserMultiplier, "group_name": frozen.Snapshot.GroupName}
				if len(frozen.Calculation.UserPayable) > 0 {
					receipt["user_breakdown"] = frozen.Calculation.UserPayable
				}
				if r.Charge.Source == "subscription" {
					receipt["subscription_multiplier"] = frozen.SubscriptionMultipliers[facts.Group]
				}
				if !q.EndUser {
					receipt["tenant_multiplier"] = frozen.TenantMultiplier
				}
				r.Pricing, _ = json.Marshal(receipt)
			}
		}
		if r.Charge.State != "posted" && r.Charge.State != "refunded" {
			r.Charge.SubscriptionUsed = 0
			r.Charge.APIKeyUsed = 0
		}
		out.Records = append(out.Records, r)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Records) > q.Limit {
		out.Records = out.Records[:q.Limit]
		last := out.Records[len(out.Records)-1]
		raw, _ := json.Marshal(recordCursor{At: last.CreatedAt, ID: last.RequestID, Kind: q.Kind})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, nil
}
func (s *RequestStore) RecordSummary(ctx context.Context, q domain.RecordQuery) (domain.RecordSummary, error) {
	var r domain.RecordSummary
	var tc, tr int64
	err := s.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE s.reason IN ('request_error_waived','unconfirmed_execution')),
 count(*) FILTER(WHERE s.delivery_interrupted),
 COALESCE(sum((s.evidence->'usage'->'reported_fields'->>'input_tokens')::bigint),0),COALESCE(sum((s.evidence->'usage'->'reported_fields'->>'output_tokens')::bigint),0),
 COALESCE(sum(s.tenant_charged),0),COALESCE(sum(s.user_charged),0),COALESCE(sum(s.tenant_charged) FILTER(WHERE s.state='refunded'),0),COALESCE(sum(s.user_charged) FILTER(WHERE s.state='refunded'),0),
 COALESCE(sum((s.evidence->'usage'->'reported_fields'->>'cache_read_tokens')::bigint),0),
 COALESCE(sum((s.evidence->'usage'->'reported_fields'->>'cache_write_tokens')::bigint),0),
 COALESCE(sum(COALESCE((s.evidence->'usage'->'reported_fields'->>'input_tokens')::bigint,0) + COALESCE((s.evidence->'usage'->'reported_fields'->>'output_tokens')::bigint,0)
 + CASE WHEN s.evidence->'usage'->>'protocol'='anthropic_messages' THEN COALESCE((s.evidence->'usage'->'reported_fields'->>'cache_read_tokens')::bigint,0)+COALESCE((s.evidence->'usage'->'reported_fields'->>'cache_write_tokens')::bigint,0) ELSE 0 END
 + CASE WHEN s.evidence->'usage'->>'protocol' IN ('gemini_generate','gemini_embeddings') THEN COALESCE((s.evidence->'usage'->'reported_fields'->>'reasoning_tokens')::bigint,0) ELSE 0 END),0),
 count(*) FILTER(WHERE COALESCE(s.evidence->'usage'->'reported_fields','{}'::jsonb)<>'{}'::jsonb),
 avg((s.dimensions->>'request_total_ms')::float8) FILTER(WHERE (s.dimensions->>'request_total_ms')::float8>0),
 avg((s.dimensions->>'first_token_latency_ms')::float8) FILTER(WHERE (s.dimensions->>'first_token_latency_ms')::float8>0),
 count(*) FILTER(WHERE (s.dimensions->>'request_total_ms')::float8>0),
 count(*) FILTER(WHERE (s.dimensions->>'first_token_latency_ms')::float8>0)
 FROM bill_settlements s LEFT JOIN ai_requests r ON r.request_id=s.request_id AND r.created_at=s.created_at
 WHERE ($1='' OR s.tenant_id=$1) AND ($2='' OR s.user_id=$2) AND ($3::timestamptz IS NULL OR s.created_at >= $3) AND ($4::timestamptz IS NULL OR s.created_at<$4)
 AND ($5='' OR s.dimensions->>'model_code'=$5) AND ($6='' OR s.dimensions->>'request_source'=$6)`+recordSearchSQL(7), q.TenantID, q.UserID, q.From, q.To, q.Model, q.Source, q.TenantName, q.UserName, q.Group, q.APIKeyName, q.RequestID).Scan(&r.Requests, &r.Errors, &r.Interruptions, &r.InputTokens, &r.OutputTokens, &tc, &r.UserCharged, &tr, &r.UserRefunded, &r.CacheReadTokens, &r.CacheWriteTokens, &r.TotalTokens, &r.TokenSamples, &r.AvgTotalMs, &r.AvgFirstTokenMs, &r.TimingSamples, &r.FirstTokenSamples)
	if err != nil {
		return r, fmt.Errorf("record summary: %w", err)
	}
	if !q.EndUser {
		r.TenantCharged = &tc
		r.TenantRefunded = &tr
	}
	return r, nil
}

var _ domain.RequestRecordRepository = (*RequestStore)(nil)
