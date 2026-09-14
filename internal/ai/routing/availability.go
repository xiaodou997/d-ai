package routing

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"xiaodou/dai/internal/ai/domain"
)

type AvailabilityPhase string

const (
	Available  AvailabilityPhase = "available"
	Cooling    AvailabilityPhase = "cooling"
	Recovering AvailabilityPhase = "recovering"
)

var ErrAvailabilityUnavailable = errors.New("availability state service unavailable")

// FaultScope identifies the smallest physical resource affected by a verdict.
// A group is deliberately absent: every tenant observes the same upstream.
type FaultScope struct {
	Key          string `json:"key"`
	Kind         string `json:"kind"`
	ResourceKind string `json:"resource_kind"`
	ResourceID   string `json:"resource_id"`
	EndpointID   string `json:"endpoint_id,omitempty"`
	CredentialID string `json:"credential_id,omitempty"`
	Model        string `json:"model,omitempty"`
	Operation    string `json:"operation,omitempty"`
}

func NewFaultScope(kind, resourceKind, resourceID, endpointID, credentialID, model, operation string) FaultScope {
	s := FaultScope{Kind: kind, ResourceKind: resourceKind, ResourceID: resourceID, EndpointID: endpointID, CredentialID: credentialID, Model: model, Operation: operation}
	b, _ := json.Marshal([]string{kind, resourceKind, resourceID, endpointID, credentialID, model, operation})
	s.Key = base64.RawURLEncoding.EncodeToString(b)
	return s
}

type AvailabilitySnapshot struct {
	Busy                bool              `json:"busy"`
	Scope               FaultScope        `json:"scope"`
	Phase               AvailabilityPhase `json:"phase"`
	Epoch               int64             `json:"epoch"`
	RetryAt             int64             `json:"retry_at,omitempty"`
	LeaseUntil          int64             `json:"lease_until,omitempty"`
	VerifiedAt          int64             `json:"verified_at,omitempty"`
	LastFailureAt       int64             `json:"last_failure_at,omitempty"`
	Reason              string            `json:"reason,omitempty"`
	ConsecutiveFailures int               `json:"consecutive_failures"`
	RecoverySuccesses   int               `json:"recovery_successes"`
	Trips               int               `json:"trips"`
	RecentTrips         []int64           `json:"recent_trips"`
}

func (s AvailabilitySnapshot) Blocked(now time.Time) bool {
	return s.Phase == Cooling && s.RetryAt > now.UnixMilli() || s.Phase == Recovering && s.Busy
}

type AdmissionPermit struct {
	Token     string           `json:"token"`
	Epochs    map[string]int64 `json:"epochs"`
	ExpiresAt int64            `json:"expires_at"`
}

type AdmissionDenied struct {
	RetryAt int64
	Reason  string
}

func (e *AdmissionDenied) Error() string { return "upstream temporarily unavailable: " + e.Reason }

// AvailabilityOutcome is independent of HTTP and billing. A neutral result
// always releases its own permit without asserting upstream health.
type AvailabilityOutcome struct {
	Success        bool   `json:"success"`
	FailureScope   string `json:"failure_scope,omitempty"`
	Reason         string `json:"reason,omitempty"`
	HealthFailure  bool   `json:"health_failure"`
	CooldownUntil  int64  `json:"cooldown_until,omitempty"`
	Authentication bool   `json:"authentication"`
}

type Availability interface {
	Read(context.Context, []FaultScope) (map[string]AvailabilitySnapshot, error)
	Acquire(context.Context, []FaultScope, time.Duration) (*AdmissionPermit, error)
	Complete(context.Context, *AdmissionPermit, AvailabilityOutcome) ([]AvailabilitySnapshot, error)
	List(context.Context, string, string) ([]AvailabilitySnapshot, error)
	Resume(context.Context, string, string) error
	RecoveryTurn(context.Context, string, []string, bool) (string, error)
}

//go:embed availability.lua
var availabilityLua string
var availabilityScript = redis.NewScript(availabilityLua)

// RedisAvailability runs the entire multi-scope transition atomically. Redis
// TIME, rather than a gateway's wall clock, controls cooldown and lease expiry.
type RedisAvailability struct{ client *redis.Client }

func NewRedisAvailability(client *redis.Client) *RedisAvailability {
	return &RedisAvailability{client: client}
}

func availabilityKey(scope string) string  { return "dai:availability:v2:scope:" + scope }
func resourceIndex(kind, id string) string { return "dai:availability:v2:resource:" + kind + ":" + id }

func (r *RedisAvailability) run(ctx context.Context, action string, scopes []FaultScope, token string, lease time.Duration, outcome AvailabilityOutcome) ([]byte, error) {
	if r == nil || r.client == nil {
		return nil, ErrAvailabilityUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	keys := make([]string, len(scopes))
	for i, s := range scopes {
		keys[i] = availabilityKey(s.Key)
	}
	meta, _ := json.Marshal(scopes)
	verdict, _ := json.Marshal(outcome)
	value, err := availabilityScript.Run(ctx, r.client, keys, action, token, lease.Milliseconds(), string(meta), string(verdict)).Text()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAvailabilityUnavailable, err)
	}
	return []byte(value), nil
}

func (r *RedisAvailability) Read(ctx context.Context, scopes []FaultScope) (map[string]AvailabilitySnapshot, error) {
	out := make(map[string]AvailabilitySnapshot, len(scopes))
	if len(scopes) == 0 {
		return out, nil
	}
	b, err := r.run(ctx, "read", scopes, "", 0, AvailabilityOutcome{})
	if err != nil {
		return nil, err
	}
	var records []AvailabilitySnapshot
	if err = json.Unmarshal(b, &records); err != nil {
		return nil, err
	}
	for _, rec := range records {
		out[rec.Scope.Key] = rec
	}
	return out, nil
}

func (r *RedisAvailability) Acquire(ctx context.Context, scopes []FaultScope, lease time.Duration) (*AdmissionPermit, error) {
	if lease <= 0 {
		lease = 17 * time.Minute
	}
	token := uuid.NewString()
	b, err := r.run(ctx, "acquire", scopes, token, lease, AvailabilityOutcome{})
	if err != nil {
		// A lost acknowledgement may hide a successful claim. The cancellation
		// tombstone also fences a delayed acquire arriving after this cleanup.
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = r.run(cleanup, "cancel", nil, token, lease, AvailabilityOutcome{})
		return nil, err
	}
	var result struct {
		Permit  *AdmissionPermit `json:"permit"`
		RetryAt int64            `json:"retry_at"`
		Reason  string           `json:"reason"`
	}
	if err = json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	if result.Permit == nil {
		return nil, &AdmissionDenied{RetryAt: result.RetryAt, Reason: result.Reason}
	}
	return result.Permit, nil
}

func (r *RedisAvailability) Complete(ctx context.Context, permit *AdmissionPermit, result AvailabilityOutcome) ([]AvailabilitySnapshot, error) {
	if permit == nil {
		return nil, nil
	}
	b, err := r.run(ctx, "complete", nil, permit.Token, 0, result)
	if err != nil {
		return nil, err
	}
	var records []AvailabilitySnapshot
	err = json.Unmarshal(b, &records)
	return records, err
}

func (r *RedisAvailability) List(ctx context.Context, kind, id string) ([]AvailabilitySnapshot, error) {
	if r == nil || r.client == nil {
		return nil, ErrAvailabilityUnavailable
	}
	index := "dai:availability:v2:scopes"
	if kind != "" && id != "" {
		index = resourceIndex(kind, id)
	}
	keys, err := r.client.SMembers(ctx, index).Result()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAvailabilityUnavailable, err)
	}
	if len(keys) == 0 {
		return []AvailabilitySnapshot{}, nil
	}
	pipe := r.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}
	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("%w: %v", ErrAvailabilityUnavailable, err)
	}
	scopes := make([]FaultScope, 0, len(keys))
	for _, c := range cmds {
		b, e := c.Bytes()
		if e == redis.Nil {
			continue
		}
		if e != nil {
			return nil, e
		}
		var rec AvailabilitySnapshot
		if e = json.Unmarshal(b, &rec); e != nil {
			return nil, e
		}
		scopes = append(scopes, rec.Scope)
	}
	states, err := r.Read(ctx, scopes)
	if err != nil {
		return nil, err
	}
	out := make([]AvailabilitySnapshot, 0, len(states))
	for _, rec := range states {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Scope.Key < out[j].Scope.Key })
	return out, nil
}

func (r *RedisAvailability) Resume(ctx context.Context, kind, id string) error {
	states, err := r.List(ctx, kind, id)
	if err != nil {
		return err
	}
	scopes := make([]FaultScope, 0, len(states))
	for _, s := range states {
		scopes = append(scopes, s.Scope)
	}
	if len(scopes) == 0 {
		return nil
	}
	_, err = r.run(ctx, "resume", scopes, "", 0, AvailabilityOutcome{})
	return err
}

var recoveryTurnScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1]); redis.call('EXPIRE', KEYS[1], 3600)
if ARGV[1] == '1' and (n-1)%10 ~= 0 then return -1 end
local p = redis.call('INCR', KEYS[2]); redis.call('EXPIRE', KEYS[2], 3600)
return (p-1)%tonumber(ARGV[2])`)

func (r *RedisAvailability) RecoveryTurn(ctx context.Context, tier string, candidates []string, hasAvailable bool) (string, error) {
	if len(candidates) == 0 {
		return "", nil
	}
	if r == nil || r.client == nil {
		return "", ErrAvailabilityUnavailable
	}
	sort.Strings(candidates)
	flag := 0
	if hasAvailable {
		flag = 1
	}
	n, err := recoveryTurnScript.Run(ctx, r.client, []string{"dai:availability:v2:turn:" + tier, "dai:availability:v2:cursor:" + tier}, flag, len(candidates)).Int()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAvailabilityUnavailable, err)
	}
	if n < 0 {
		return "", nil
	}
	return candidates[n], nil
}

// Reset starts a new generation for only the configuration scopes that changed.
func (r *RedisAvailability) Reset(ctx context.Context, scopes []FaultScope) error {
	if len(scopes) == 0 {
		return nil
	}
	_, err := r.run(ctx, "reset", scopes, "", 0, AvailabilityOutcome{})
	return err
}

// ImportCooldown is idempotent and used only by the maintenance cutover.
func (r *RedisAvailability) ImportCooldown(ctx context.Context, scope FaultScope, until time.Time, reason string) error {
	_, err := r.run(ctx, "import", []FaultScope{scope}, "", 0, AvailabilityOutcome{CooldownUntil: until.UnixMilli(), Reason: reason})
	return err
}

// Suspend records explicit credential rejection from token refresh/diagnostics.
func (r *RedisAvailability) Suspend(ctx context.Context, scope FaultScope, until time.Time, reason string) error {
	_, err := r.run(ctx, "suspend", []FaultScope{scope}, "", 0, AvailabilityOutcome{CooldownUntil: until.UnixMilli(), Reason: reason})
	return err
}

// CandidateScopes is shared by credential selection and execution admission.
func CandidateScopes(c *domain.RouteCandidate, cred *domain.OAuthCredential) []FaultScope {
	if c == nil {
		return nil
	}
	kind, resource, endpoint, credential := "direct_upstream", c.EffectiveAccountID(), c.EndpointID, ""
	if c.IsPoolRoute() {
		kind, resource, endpoint = "oauth_pool", c.PoolID, ""
		if cred != nil {
			credential = cred.ID
		}
	}
	scopes := []FaultScope{NewFaultScope("transport", kind, resource, endpoint, "", "", "")}
	if !c.IsPoolRoute() || cred != nil {
		scopes = append(scopes, NewFaultScope("authentication", kind, resource, "", credential, "", ""), NewFaultScope("model", kind, resource, endpoint, credential, c.EffectiveUpstreamModel(), c.OperationKey()), NewFaultScope("rate_limit", kind, resource, endpoint, credential, c.EffectiveUpstreamModel(), c.OperationKey()))
	}
	return scopes
}
