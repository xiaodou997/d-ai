package transport

import (
	"errors"
	"net/http"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

func TestMapServiceErrorUsesDomainPersistenceErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantDetail string
	}{
		{name: "not found", err: domain.ErrNotFound, wantStatus: http.StatusNotFound, wantDetail: "resource not found"},
		{name: "unique", err: &domain.PersistenceError{Kind: domain.PersistenceConflict}, wantStatus: http.StatusConflict, wantDetail: "resource already exists"},
		{name: "foreign key", err: &domain.PersistenceError{Kind: domain.PersistenceReferenceNotFound}, wantStatus: http.StatusBadRequest, wantDetail: "referenced resource not found"},
		{name: "check", err: &domain.PersistenceError{Kind: domain.PersistenceInvalidField}, wantStatus: http.StatusBadRequest, wantDetail: "invalid field value"},
		{name: "format", err: &domain.PersistenceError{Kind: domain.PersistenceInvalidFormat}, wantStatus: http.StatusBadRequest, wantDetail: "invalid input format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapServiceError(tt.err)
			var appErr *httpx.AppError
			if !errors.As(got, &appErr) || appErr.Status != tt.wantStatus || appErr.Detail != tt.wantDetail {
				t.Fatalf("error = %#v, want status %d detail %q", got, tt.wantStatus, tt.wantDetail)
			}
		})
	}
}
