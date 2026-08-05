package console

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/domain"
)

// writeServiceErr maps a service-layer error onto the package {code,message}
// envelope. It checks domain sentinels first, then falls back to the same
// pgx/pgconn mapping used by writeDBErr (so repos that pass raw DB errors
// through still produce the correct 404/409/400 instead of a blanket 500).
// Truly unknown errors are logged and returned as a generic 500.
func (s *Console) writeServiceErr(w http.ResponseWriter, r *http.Request, err error) {
	var verr *domain.ValidationError
	switch {
	case errors.As(err, &verr):
		msg := verr.Message
		if verr.Field != "" {
			msg = verr.Field + ": " + verr.Message
		}
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, msg)
		return
	case errors.Is(err, domain.ErrValidation):
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, err.Error())
		return
	case errors.Is(err, domain.ErrNotFound):
		writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
		return
	case errors.Is(err, domain.ErrConflict):
		writeErr(w, http.StatusConflict, BizErrConflict, "resource already exists")
		return
	case errors.Is(err, domain.ErrForbidden):
		writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
		return
	}

	// Fallback: preserve the existing writeDBErr contract for repos that surface
	// raw pgx/pgconn errors. IMPORTANT: pgErr.Message can leak column/constraint
	// names, so only the error class is exposed; full detail is in the logs.
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
		s.logger.Error("service db error",
			consoleRequestLogFields(r, zap.Error(err))...,
		)
		writeErr(w, http.StatusInternalServerError, BizErrDatabase, "database error")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, BizErrNotFound, "not found")
		return
	}

	s.logger.Error("service error",
		consoleRequestLogFields(r, zap.Error(err))...,
	)
	writeErr(w, http.StatusInternalServerError, BizErrInternal, "server error")
}
