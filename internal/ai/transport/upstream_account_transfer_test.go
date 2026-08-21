package transport

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/libs/go/httpx"
)

func TestValidateImportPriceBook(t *testing.T) {
	tests := []struct {
		name       string
		priceBook  string
		reader     PriceBookReader
		wantDetail string
	}{
		{name: "empty is optional"},
		{name: "reader unavailable", priceBook: "book-1", wantDetail: "price book reader is not configured"},
		{name: "exists", priceBook: "book-1", reader: &priceBookValidationReader{}},
		{name: "not found", priceBook: "book-1", reader: &priceBookValidationReader{err: domain.ErrNotFound}, wantDetail: "default_price_book_id does not exist"},
		{name: "invalid", priceBook: "not-a-uuid", reader: &priceBookValidationReader{err: errors.New("invalid UUID")}, wantDetail: "invalid default_price_book_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateImportPriceBook(context.Background(), tt.reader, tt.priceBook)
			if tt.wantDetail == "" {
				if err != nil {
					t.Fatalf("validateImportPriceBook error = %v", err)
				}
				return
			}
			var appErr *httpx.AppError
			if !errors.As(err, &appErr) || appErr.Detail != tt.wantDetail {
				t.Fatalf("error = %#v, want detail %q", err, tt.wantDetail)
			}
		})
	}
}

type priceBookValidationReader struct {
	err error
}

func (r *priceBookValidationReader) GetPriceBook(context.Context, string) (domain.PriceBook, error) {
	return domain.PriceBook{}, r.err
}
