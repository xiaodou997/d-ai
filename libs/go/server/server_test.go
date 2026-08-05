package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"xiaodou/dai/libs/go/httpx"
)

type helloOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func newTestAPI(t *testing.T) (*httptest.Server, huma.API) {
	t.Helper()
	r, api := New(Options{Title: "test", Version: "0.0.0"})

	// 正常 200 强类型
	huma.Register(api, huma.Operation{OperationID: "hello", Method: http.MethodGet, Path: "/hello"},
		func(ctx context.Context, _ *struct{}) (*helloOutput, error) {
			out := &helloOutput{}
			out.Body.Message = "hi"
			return out, nil
		})

	// 业务错误
	huma.Register(api, huma.Operation{OperationID: "missing", Method: http.MethodGet, Path: "/missing"},
		func(ctx context.Context, _ *struct{}) (*helloOutput, error) {
			return nil, httpx.ErrNotFound.WithDetail("nope")
		})

	// 输入校验（必填 query）
	type validIn struct {
		Name string `query:"name" required:"true"`
	}
	huma.Register(api, huma.Operation{OperationID: "needs-name", Method: http.MethodGet, Path: "/needs-name"},
		func(ctx context.Context, _ *validIn) (*helloOutput, error) {
			return &helloOutput{}, nil
		})

	// panic
	huma.Register(api, huma.Operation{OperationID: "boom", Method: http.MethodGet, Path: "/boom"},
		func(ctx context.Context, _ *struct{}) (*helloOutput, error) {
			panic("kaboom")
		})

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts, api
}

func decodeProblem(t *testing.T, resp *http.Response) httpx.Problem {
	t.Helper()
	var p httpx.Problem
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return p
}

func TestBase_OKStrongTyped(t *testing.T) {
	ts, _ := newTestAPI(t)
	resp, err := http.Get(ts.URL + "/hello")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Message != "hi" {
		t.Errorf("message = %q, want hi", body.Message)
	}
}

func TestBase_BusinessErrorIsProblemJSON(t *testing.T) {
	ts, _ := newTestAPI(t)
	resp, err := http.Get(ts.URL + "/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != httpx.ProblemContentType {
		t.Fatalf("content-type = %q, want %q", ct, httpx.ProblemContentType)
	}
	p := decodeProblem(t, resp)
	if p.Code != "not_found" || p.Status != 404 || p.Detail != "nope" {
		t.Errorf("unexpected problem: %+v", p)
	}
}

func TestBase_HumaValidationIsProblemJSON(t *testing.T) {
	ts, _ := newTestAPI(t)
	resp, err := http.Get(ts.URL + "/needs-name") // 缺必填 name
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	p := decodeProblem(t, resp)
	if p.Code != "validation_failed" {
		t.Errorf("code = %q, want validation_failed", p.Code)
	}
	if len(p.Errors) == 0 {
		t.Errorf("expected field errors, got none")
	}
}

func TestBase_PanicRecoveredAsProblemJSON(t *testing.T) {
	ts, _ := newTestAPI(t)
	resp, err := http.Get(ts.URL + "/boom")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != httpx.ProblemContentType {
		t.Fatalf("content-type = %q, want %q", ct, httpx.ProblemContentType)
	}
	p := decodeProblem(t, resp)
	if p.Code != "internal" {
		t.Errorf("code = %q, want internal", p.Code)
	}
	if p.RequestID == "" {
		t.Errorf("expected request_id to be set")
	}
}

func TestBase_RequestLogIncludesErrorCause(t *testing.T) {
	core, observed := observer.New(zapcore.InfoLevel)
	log := zap.New(core)
	r, api := New(Options{Title: "test", Version: "0.0.0", Logger: log})
	huma.Register(api, huma.Operation{OperationID: "db-fail", Method: http.MethodGet, Path: "/db-fail"},
		func(ctx context.Context, _ *struct{}) (*helloOutput, error) {
			return nil, httpx.ErrInternal.WithCause(errors.New("scan row: missing destination name subscription_sale_note"))
		})

	ts := httptest.NewServer(r)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/db-fail")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	p := decodeProblem(t, resp)
	if strings.Contains(p.Detail, "subscription_sale_note") {
		t.Fatalf("response leaked internal detail: %+v", p)
	}

	var accessLog observer.LoggedEntry
	found := false
	for _, entry := range observed.AllUntimed() {
		if entry.Message == "HTTP Request" {
			accessLog = entry
			found = true
		}
	}
	if !found {
		t.Fatal("missing HTTP Request log")
	}
	if got := accessLog.ContextMap()["error"]; !strings.Contains(got.(string), "subscription_sale_note") {
		t.Fatalf("log error = %#v, want cause detail", got)
	}
	if got := accessLog.ContextMap()["error_cause"]; got != "scan row: missing destination name subscription_sale_note" {
		t.Fatalf("log error_cause = %#v", got)
	}
}
