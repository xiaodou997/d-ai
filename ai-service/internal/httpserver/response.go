package httpserver

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// APIResponse is the standard envelope for all management API responses.
type APIResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// PageData wraps paginated list results.
type PageData struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// writeOK writes HTTP 200 with code:0 and the given data payload.
func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, APIResponse{Code: BizOK, Message: "success", Data: data})
}

// writeErr writes an HTTP error response with the given biz code and message.
func writeErr(w http.ResponseWriter, httpStatus int, bizCode int, msg string) {
	writeJSON(w, httpStatus, APIResponse{Code: bizCode, Message: msg, Data: nil})
}

// writeDBErr maps pgx/pgconn errors to the appropriate HTTP + biz code response.
// IMPORTANT: pgErr.Message often contains column names, constraint names, or
// table internals that would leak schema details to the client. We only expose
// the error class via a generic message and rely on the caller's logging
// middleware to capture the full error.
func writeDBErr(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			writeErr(w, http.StatusConflict, BizErrConflict, "resource already exists")
			return
		case "23503":
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "referenced resource not found")
			return
		case "23514":
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid field value")
			return
		case "22P02":
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid input format")
			return
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
		return
	}
	writeErr(w, http.StatusInternalServerError, BizErrDatabase, "database error")
}

