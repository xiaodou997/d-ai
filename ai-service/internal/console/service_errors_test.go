package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"xiaodou/unihub/ai-service/internal/domain"
)

func decodeEnvelope(t *testing.T, body []byte) APIResponse {
	t.Helper()
	var env APIResponse
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("response is not a {code,message,data} envelope: %v (body=%s)", err, body)
	}
	return env
}

// TestWriteServiceErr_PreservesContract locks the error-contract: domain
// sentinels AND raw pgx/pgconn errors map to the same HTTP status / biz code the
// legacy writeDBErr produced — guarding against the regression where everything
// degraded to 500.
func TestWriteServiceErr_PreservesContract(t *testing.T) {
	c := &Console{logger: zap.NewNop()}
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   int
	}{
		{"validation field", domain.NewValidationError("name", "name is required"), http.StatusBadRequest, BizErrBadRequest},
		{"validation sentinel", fmt.Errorf("wrap: %w", domain.ErrValidation), http.StatusBadRequest, BizErrBadRequest},
		{"not found", domain.ErrNotFound, http.StatusNotFound, BizErrNotFound},
		{"conflict", domain.ErrConflict, http.StatusConflict, BizErrConflict},
		{"forbidden", domain.ErrForbidden, http.StatusForbidden, BizErrForbidden},
		{"pg unique", &pgconn.PgError{Code: "23505"}, http.StatusConflict, BizErrConflict},
		{"pg fk", &pgconn.PgError{Code: "23503"}, http.StatusBadRequest, BizErrBadRequest},
		{"pg check", &pgconn.PgError{Code: "23514"}, http.StatusBadRequest, BizErrBadRequest},
		{"pg invalid text", &pgconn.PgError{Code: "22P02"}, http.StatusBadRequest, BizErrBadRequest},
		{"pg other", &pgconn.PgError{Code: "55000"}, http.StatusInternalServerError, BizErrDatabase},
		{"no rows", pgx.ErrNoRows, http.StatusNotFound, BizErrNotFound},
		{"wrapped no rows", fmt.Errorf("query: %w", pgx.ErrNoRows), http.StatusNotFound, BizErrNotFound},
		{"unknown", errors.New("boom"), http.StatusInternalServerError, BizErrInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
			c.writeServiceErr(rec, req, tc.err)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d want %d", rec.Code, tc.wantStatus)
			}
			env := decodeEnvelope(t, rec.Body.Bytes())
			if env.Code != tc.wantCode {
				t.Fatalf("biz code: got %d want %d", env.Code, tc.wantCode)
			}
		})
	}
}

// TestRecoverer_KeepsEnvelope locks Finding 3: a panic in a management handler
// still yields the {code,message,data} envelope, not httpx's plain 500.
func TestRecoverer_KeepsEnvelope(t *testing.T) {
	c := &Console{logger: zap.NewNop()}
	h := c.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/x", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d want 500", rec.Code)
	}
	env := decodeEnvelope(t, rec.Body.Bytes())
	if env.Code != BizErrInternal {
		t.Fatalf("biz code: got %d want %d", env.Code, BizErrInternal)
	}
}

// TestProviderEndpointDTO_ExtraHeaders documents Finding 2's contract at the
// mapper boundary: a populated ExtraHeaders renders as a JSON object, and the
// repo must backfill it (nil would wrongly render null).
func TestProviderEndpointDTO_ExtraHeaders(t *testing.T) {
	withHeaders := providerEndpointFromDomain(domain.ProviderEndpoint{ExtraHeaders: []byte(`{"X-Foo":"bar"}`)})
	b, _ := json.Marshal(withHeaders.ExtraHeaders)
	if string(b) != `{"X-Foo":"bar"}` {
		t.Fatalf("populated extra_headers must pass through, got %s", b)
	}
	empty := providerEndpointFromDomain(domain.ProviderEndpoint{ExtraHeaders: []byte("{}")})
	b2, _ := json.Marshal(empty.ExtraHeaders)
	if string(b2) != `{}` {
		t.Fatalf("empty-object extra_headers must stay {}, got %s", b2)
	}
}
