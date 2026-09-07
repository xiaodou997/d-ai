package riskcontrol

import (
	"context"
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

type fakeSettingRepo struct {
	value    json.RawMessage
	getCalls int
	getErr   error
}

func (r *fakeSettingRepo) GetSetting(context.Context, string) (json.RawMessage, error) {
	r.getCalls++
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.value, nil
}

func (r *fakeSettingRepo) UpsertVersionedSetting(_ context.Context, _ string, value json.RawMessage, expectedRevision int64) (json.RawMessage, error) {
	var current, next domain.RiskControlConfig
	_ = json.Unmarshal(r.value, &current)
	if current.ConfigRevision != expectedRevision {
		return nil, domain.ErrConflict
	}
	if err := json.Unmarshal(value, &next); err != nil {
		return nil, err
	}
	next.ConfigRevision = current.ConfigRevision + 1
	r.value, _ = json.Marshal(next)
	return r.value, nil
}

func TestConfigService_GetFallsBackWhenMissing(t *testing.T) {
	repo := &fakeSettingRepo{getErr: domain.ErrNotFound}
	svc := NewConfigService(repo)
	cfg, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.Mode != domain.RiskControlModeOff {
		t.Fatalf("expected disabled default, got %#v", cfg)
	}
}

func TestConfigService_UpdateThenGetReflectsChange(t *testing.T) {
	repo := &fakeSettingRepo{}
	svc := NewConfigService(repo)
	want := domain.RiskControlConfig{
		Enabled: true, Mode: domain.RiskControlModeObserve,
		Keyword:    domain.KeywordConfig{Enabled: true},
		SampleRate: 1,
	}
	if _, err := svc.Update(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Mode != domain.RiskControlModeObserve {
		t.Fatalf("got %#v", got)
	}
	if len(got.Thresholds) != len(domain.DefaultRiskControlThresholds()) {
		t.Fatalf("thresholds not backfilled: %#v", got.Thresholds)
	}
}

func TestConfigService_UpdateRejectsInvalidMode(t *testing.T) {
	repo := &fakeSettingRepo{}
	svc := NewConfigService(repo)
	_, err := svc.Update(context.Background(), domain.RiskControlConfig{Mode: "bogus"})
	if err == nil {
		t.Fatal("expected validation error for invalid mode")
	}
}

func TestConfigService_UpdateRejectsInvalidSampleRate(t *testing.T) {
	repo := &fakeSettingRepo{}
	svc := NewConfigService(repo)
	cfg := domain.RiskControlConfig{Mode: domain.RiskControlModeOff, SampleRate: 1.5}
	if _, err := svc.Update(context.Background(), cfg); err == nil {
		t.Fatal("expected validation error for sample_rate > 1")
	}
}

func TestConfigService_UpdateBackfillsZeroValueDefaults(t *testing.T) {
	repo := &fakeSettingRepo{}
	svc := NewConfigService(repo)
	cfg := domain.RiskControlConfig{Mode: domain.RiskControlModeOff}
	if _, err := svc.Update(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.ViolationWindowHours != 24 || got.RiskEventThreshold != 3 || got.BlockStatusCode != 451 {
		t.Fatalf("defaults not backfilled: %#v", got)
	}
}

func TestConfigService_CachesWithinTTL(t *testing.T) {
	repo := &fakeSettingRepo{}
	svc := NewConfigService(repo)
	if _, err := svc.Update(context.Background(), domain.RiskControlConfig{Mode: domain.RiskControlModeOff}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := repo.getCalls
	if _, err := svc.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.getCalls != callsAfterFirst {
		t.Fatalf("expected cached Get to skip the repo, calls %d -> %d", callsAfterFirst, repo.getCalls)
	}
}

func TestConfigService_UpdateBumpsConfigRevision(t *testing.T) {
	repo := &fakeSettingRepo{}
	svc := NewConfigService(repo)
	cfg := domain.RiskControlConfig{Mode: domain.RiskControlModeOff}
	if _, err := svc.Update(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(context.Background())
	if got.ConfigRevision != 1 {
		t.Fatalf("expected revision 1 after first update, got %d", got.ConfigRevision)
	}
	// Second update should bump to 2.
	cfg = got
	if _, err := svc.Update(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Get(context.Background())
	if got.ConfigRevision != 2 {
		t.Fatalf("expected revision 2 after second update, got %d", got.ConfigRevision)
	}
}

func TestConfigServiceReturnsStorageAndDecodeFailures(t *testing.T) {
	for _, repo := range []*fakeSettingRepo{
		{getErr: context.DeadlineExceeded},
		{value: json.RawMessage(`{"broken":`)},
	} {
		if _, err := NewConfigService(repo).Get(context.Background()); err == nil {
			t.Fatal("configuration failure was silently converted to off")
		}
	}
}

// A read started before Update may finish afterwards; it must not repopulate
// the cache with its old snapshot after Update invalidated it.
type delayedConfigRepo struct {
	fakeSettingRepo
	readStarted chan struct{}
	releaseRead chan struct{}
	delay       bool
}

func (r *delayedConfigRepo) GetSetting(ctx context.Context, key string) (json.RawMessage, error) {
	raw, err := r.fakeSettingRepo.GetSetting(ctx, key)
	if r.delay {
		r.delay = false
		close(r.readStarted)
		<-r.releaseRead
	}
	return raw, err
}
func TestConfigServiceInflightReadCannotRestoreOldCache(t *testing.T) {
	repo := &delayedConfigRepo{
		fakeSettingRepo: fakeSettingRepo{value: json.RawMessage(`{"mode":"off","config_revision":1}`)},
		readStarted:     make(chan struct{}), releaseRead: make(chan struct{}), delay: true,
	}
	svc := NewConfigService(repo)
	done := make(chan error, 1)
	go func() { _, err := svc.Get(context.Background()); done <- err }()
	<-repo.readStarted
	if _, err := svc.Update(context.Background(), domain.RiskControlConfig{Mode: domain.RiskControlModeObserve, ConfigRevision: 1}); err != nil {
		t.Fatal(err)
	}
	close(repo.releaseRead)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	cfg, err := svc.Get(context.Background())
	if err != nil || cfg.ConfigRevision != 2 {
		t.Fatalf("stale cache: revision=%d err=%v", cfg.ConfigRevision, err)
	}
}
