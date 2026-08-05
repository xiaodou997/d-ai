package clientruntime

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultCredentialCooldown = 5 * time.Minute

func responseCooldownUntil(headers http.Header, observedAt time.Time) time.Time {
	if headers != nil {
		retryAfter := strings.TrimSpace(headers.Get("Retry-After"))
		if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
			return observedAt.Add(time.Duration(seconds) * time.Second)
		}
		if retryAt, err := http.ParseTime(retryAfter); err == nil && retryAt.After(observedAt) {
			return retryAt
		}
	}
	return observedAt.Add(defaultCredentialCooldown)
}
