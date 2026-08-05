package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteProblem_ContentTypeAndDefaults(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteProblem(rec, Problem{Status: http.StatusBadGateway})

	if got := rec.Header().Get("Content-Type"); got != ProblemContentType {
		t.Fatalf("content-type = %q, want %q", got, ProblemContentType)
	}
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// type 省略时按 RFC 7807 视为 about:blank，故响应体中应无该字段。
	if p.Type != "" {
		t.Errorf("Type = %q, want empty (omitted)", p.Type)
	}
	if p.Title != http.StatusText(http.StatusBadGateway) {
		t.Errorf("Title = %q, want %q", p.Title, http.StatusText(http.StatusBadGateway))
	}
}

func TestAppError_Problem(t *testing.T) {
	ae := ErrNotFound.WithDetail("user 42 missing")
	p := ae.Problem("req-123")

	if p.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want 404", p.Status)
	}
	if p.Code != "not_found" {
		t.Errorf("Code = %q, want not_found", p.Code)
	}
	if p.Detail != "user 42 missing" {
		t.Errorf("Detail = %q", p.Detail)
	}
	if p.RequestID != "req-123" {
		t.Errorf("RequestID = %q", p.RequestID)
	}
}

// WithXxx 派生必须返回副本，绝不污染共享模板。
func TestAppError_DerivationDoesNotMutateTemplate(t *testing.T) {
	_ = ErrValidation.
		WithDetail("bad email").
		WithFields(FieldError{Field: "email", Message: "invalid"}).
		WithMeta(map[string]any{"retry_after": 60}).
		WithCause(errors.New("boom"))

	if ErrValidation.Detail != "" {
		t.Errorf("template Detail polluted: %q", ErrValidation.Detail)
	}
	if len(ErrValidation.Fields) != 0 {
		t.Errorf("template Fields polluted: %v", ErrValidation.Fields)
	}
	if ErrValidation.cause != nil {
		t.Errorf("template cause polluted: %v", ErrValidation.cause)
	}
	if ErrValidation.Meta != nil {
		t.Errorf("template Meta polluted: %v", ErrValidation.Meta)
	}
}

func TestAppError_MetaIsCopiedIntoProblem(t *testing.T) {
	meta := map[string]any{"retry_after": 60}
	ae := ErrConflict.WithMeta(meta)
	meta["retry_after"] = 1
	p := ae.Problem("req-meta")
	if got := p.Meta["retry_after"]; got != 60 {
		t.Fatalf("retry_after = %v, want 60", got)
	}
}

func TestAppError_GetStatusAndUnwrap(t *testing.T) {
	cause := errors.New("db down")
	ae := ErrInternal.WithCause(cause)

	if ae.GetStatus() != http.StatusInternalServerError {
		t.Errorf("GetStatus = %d, want 500", ae.GetStatus())
	}
	if !errors.Is(ae, cause) {
		t.Errorf("errors.Is should find wrapped cause")
	}
}

func TestProblemFrom(t *testing.T) {
	t.Run("AppError 按其渲染", func(t *testing.T) {
		p := ProblemFrom(ErrForbidden.WithDetail("nope"), "r1")
		if p.Status != http.StatusForbidden || p.Code != "forbidden" || p.Detail != "nope" {
			t.Fatalf("unexpected problem: %+v", p)
		}
	})
	t.Run("包装的 AppError 也能被识别", func(t *testing.T) {
		wrapped := fmt.Errorf("layer: %w", ErrConflict)
		p := ProblemFrom(wrapped, "r2")
		if p.Status != http.StatusConflict {
			t.Fatalf("status = %d, want 409", p.Status)
		}
	})
	t.Run("未知 error 回退 500 且不泄露细节", func(t *testing.T) {
		p := ProblemFrom(errors.New("secret internals"), "r3")
		if p.Status != http.StatusInternalServerError || p.Code != "internal" {
			t.Fatalf("unexpected problem: %+v", p)
		}
		if p.Detail != "" {
			t.Errorf("Detail should be empty, got %q", p.Detail)
		}
	})
}

func TestNewPage_NilBecomesEmptySlice(t *testing.T) {
	p := NewPage[int](nil, 0, 1, 20)
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(raw["items"]) != "[]" {
		t.Errorf("items = %s, want []", raw["items"])
	}
}
