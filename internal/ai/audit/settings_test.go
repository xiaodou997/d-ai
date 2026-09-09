package audit

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

type recordingRepo struct {
	raw json.RawMessage
	err error
}

func (r *recordingRepo) GetSetting(context.Context, string) (json.RawMessage, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.raw == nil {
		return nil, domain.ErrNotFound
	}
	return r.raw, nil
}
func (r *recordingRepo) UpsertSetting(_ context.Context, _ string, raw json.RawMessage) error {
	if r.err != nil {
		return r.err
	}
	r.raw = append([]byte(nil), raw...)
	return nil
}

func TestRecordingSettingsPersistence(t *testing.T) {
	ctx := context.Background()
	repo := &recordingRepo{}
	svc := NewRecordingSettingsService(repo)
	c, err := svc.Get(ctx)
	if err != nil || c.Level != "basic" {
		t.Fatalf("default: %+v %v", c, err)
	}
	c, err = svc.Update(ctx, RecordingSettings{Level: "headers", SensitiveHeaders: []string{" X-Private ", "x-private"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(c.SensitiveHeaders, "authorization") || !slices.Contains(c.SensitiveHeaders, "x-private") {
		t.Fatalf("redaction: %+v", c)
	}
	c, err = svc.Get(ctx)
	if err != nil || c.Level != "headers" {
		t.Fatalf("cache not invalidated: %+v %v", c, err)
	}
	c, err = NewRecordingSettingsService(repo).Get(ctx)
	if err != nil || c.Level != "headers" {
		t.Fatalf("not durable: %+v %v", c, err)
	}
	if _, err = svc.Update(ctx, RecordingSettings{Level: "invalid"}); err == nil {
		t.Fatal("invalid accepted")
	}
	if _, err = svc.Update(ctx, RecordingSettings{Level: "full", SensitiveHeaders: []string{"x\nsecret"}}); err == nil {
		t.Fatal("invalid header accepted")
	}
}

func TestRecordingSettingsReadFailureIsNotFull(t *testing.T) {
	svc := NewRecordingSettingsService(&recordingRepo{err: errors.New("unavailable")})
	if _, err := svc.Get(context.Background()); err == nil {
		t.Fatal("read failure hidden")
	}
}
