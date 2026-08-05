package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestIndustryStandardRuntimeRouteContract(t *testing.T) {
	router := chi.NewRouter()
	(&Gateway{}).Routes(router)

	registered := make(map[string]struct{})
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		registered[method+" "+route] = struct{}{}
		return nil
	}); err != nil {
		t.Fatalf("walk runtime routes: %v", err)
	}

	want := []string{
		http.MethodGet + " /models",
		http.MethodPost + " /chat/completions",
		http.MethodPost + " /responses",
		http.MethodPost + " /embeddings",
		http.MethodPost + " /images/generations",
		http.MethodPost + " /images/edits",
		http.MethodPost + " /v1/tasks",
		http.MethodGet + " /v1/tasks",
		http.MethodGet + " /v1/tasks/{taskID}",
		http.MethodPost + " /v1/tasks/{taskID}/cancel",
		http.MethodGet + " /v1/models",
		http.MethodPost + " /v1/chat/completions",
		http.MethodPost + " /v1/responses",
		http.MethodPost + " /v1/embeddings",
		http.MethodPost + " /v1/images/generations",
		http.MethodPost + " /v1/images/edits",
		http.MethodPost + " /v1/messages",
		http.MethodPost + " /v1/messages/count_tokens",
		http.MethodPost + " /v1beta/models/{modelAction}",
	}
	for _, route := range want {
		if _, ok := registered[route]; !ok {
			t.Errorf("industry-standard runtime route removed or changed: %s", route)
		}
	}
}

func TestVersionlessRuntimePathsPresentCanonicalPathsToHandlers(t *testing.T) {
	tests := []struct {
		alias     string
		canonical string
	}{
		{alias: "/models", canonical: "/v1/models"},
		{alias: "/chat/completions", canonical: "/v1/chat/completions"},
		{alias: "/responses", canonical: "/v1/responses"},
		{alias: "/embeddings", canonical: "/v1/embeddings"},
		{alias: "/images/generations", canonical: "/v1/images/generations"},
		{alias: "/images/edits", canonical: "/v1/images/edits"},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			router := chi.NewRouter()
			router.With(canonicalRuntimePath(tt.canonical)).Handle(tt.alias, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path + "?" + r.URL.RawQuery))
			}))

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, tt.alias+"?trace=1", nil))

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if got, want := recorder.Body.String(), tt.canonical+"?trace=1"; got != want {
				t.Fatalf("handler URL = %q, want %q", got, want)
			}
		})
	}
}
