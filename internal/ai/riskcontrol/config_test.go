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

func (r *fakeSettingRepo) UpsertSetting(_ context.Context, _ string, value json.RawMessage) error {
	r.value = value
	return nil
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
	if err := svc.Update(context.Background(), want); err != nil {
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
	err := svc.Update(context.Background(), domain.RiskControlConfig{Mode: "bogus"})
	if err == nil {
		t.Fatal("expected validation error for invalid mode")
	}
}

func TestConfigService_UpdateRejectsInvalidSampleRate(t *testing.T) {
	repo := &fakeSettingRepo{}
	svc := NewConfigService(repo)
	cfg := domain.RiskControlConfig{Mode: domain.RiskControlModeOff, SampleRate: 1.5}
	if err := svc.Update(context.Background(), cfg); err == nil {
		t.Fatal("expected validation error for sample_rate > 1")
	}
}

func TestConfigService_UpdateBackfillsZeroValueDefaults(t *testing.T) {
	repo := &fakeSettingRepo{}
	svc := NewConfigService(repo)
	cfg := domain.RiskControlConfig{Mode: domain.RiskControlModeOff}
	if err := svc.Update(context.Background(), cfg); err != nil {
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
	if err := svc.Update(context.Background(), domain.RiskControlConfig{Mode: domain.RiskControlModeOff}); err != nil {
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
	if err := svc.Update(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(context.Background())
	if got.ConfigRevision != 1 {
		t.Fatalf("expected revision 1 after first update, got %d", got.ConfigRevision)
	}
	// Second update should bump to 2.
	if err := svc.Update(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Get(context.Background())
	if got.ConfigRevision != 2 {
		t.Fatalf("expected revision 2 after second update, got %d", got.ConfigRevision)
	}
}
