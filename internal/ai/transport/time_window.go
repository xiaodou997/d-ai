package transport

import (
	"time"

	"xiaodou/dai/libs/go/httpx"
)

func parseOptionalRFC3339Window(dateFromValue, dateToValue string) (*time.Time, *time.Time, error) {
	dateFrom, err := parseOptionalRFC3339(dateFromValue, "date_from")
	if err != nil {
		return nil, nil, err
	}
	dateTo, err := parseOptionalRFC3339(dateToValue, "date_to")
	if err != nil {
		return nil, nil, err
	}
	if dateFrom != nil && dateTo != nil && !dateFrom.Before(*dateTo) {
		return nil, nil, httpx.ErrBadRequest.WithDetail("date_to must be greater than date_from")
	}
	return dateFrom, dateTo, nil
}

func applyDefaultRFC3339Window(dateFrom, dateTo *time.Time, duration time.Duration) (*time.Time, *time.Time) {
	if dateFrom != nil || dateTo != nil {
		return dateFrom, dateTo
	}
	endAt := time.Now().UTC()
	startAt := endAt.Add(-duration)
	return &startAt, &endAt
}
