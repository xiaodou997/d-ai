package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestUserSelfControlRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/user-api-keys"},
		{http.MethodPost, "/api/v1/users/me/api-keys"},
		{http.MethodPatch, "/api/v1/users/me/api-keys/key-1"},
		{http.MethodPatch, "/api/v1/users/me/api-keys/key-1/status"},
		{http.MethodPost, "/api/v1/users/me/api-keys/key-1/reveal"},
		{http.MethodPost, "/api/v1/users/me/api-keys/key-1/rotate"},
		{http.MethodDelete, "/api/v1/users/me/api-keys/key-1"},
		{http.MethodGet, "/api/v1/users/me/api-key-limit-policies"},
		{http.MethodPut, "/api/v1/users/me/api-keys/key-1/limit-policies"},
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, AIDeps{})
	for _, route := range routes {
		recorder := performUserSelfControlRequest(coreRouter, route.method, route.path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI user self-control route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusNotFound)
		}
	}

	controlRouter, controlAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterUserSelfControl(controlAPI, UserSelfControlHTTPDeps{})
	for _, route := range routes {
		recorder := performUserSelfControlRequest(controlRouter, route.method, route.path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent user self-control route %s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performUserSelfControlRequest(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}
