package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaodou/dai/libs/go/server"
)

func TestUserSelfReadRoutesRegisterIndependentlyFromCoreAI(t *testing.T) {
	routes := []string{
		"/api/v1/users/me/groups",
		"/api/v1/users/me/groups/group-1/effective-prices",
		"/api/v1/user-model-grants",
		"/api/v1/user-usage-logs",
		"/api/v1/user-usage-summary",
	}

	coreRouter, coreAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterAICore(coreAPI, CoreHTTPDeps{})
	for _, path := range routes {
		recorder := performUserSelfReadRequest(coreRouter, path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("core AI user self-read route %s status = %d, want %d", path, recorder.Code, http.StatusNotFound)
		}
	}

	readRouter, readAPI := server.New(server.Options{Title: "test", Version: "test"})
	RegisterUserSelfRead(readAPI, UserSelfReadHTTPDeps{})
	for _, path := range routes {
		recorder := performUserSelfReadRequest(readRouter, path)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("independent user self-read route %s status = %d, want %d", path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func performUserSelfReadRequest(handler http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}
