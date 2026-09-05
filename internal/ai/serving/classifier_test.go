package serving

import (
	"context"
	"net/http"
	"testing"
)

func TestClassifyOutcomeRetriesEvery5xx(t *testing.T) {
	t.Parallel()

	for status := 500; status <= 599; status++ {
		outcome := ClassifyOutcome(status, nil)
		if outcome.Status != ResultServerError {
			t.Fatalf("status %d classified as %s, want server_error", status, outcome.Status)
		}
		if outcome.Decision(false) != DecisionRetry {
			t.Fatalf("status %d decision = %v, want retry", status, outcome.Decision(false))
		}
		if !outcome.CountsAsHealthFailure() {
			t.Fatalf("status %d did not count as a health failure", status)
		}
	}
}

func TestClassifyOutcomeTreatsClientCancellationSeparately(t *testing.T) {
	t.Parallel()

	outcome := ClassifyOutcome(0, context.Canceled)
	if outcome.Status != ResultCanceled {
		t.Fatalf("context.Canceled classified as %s, want canceled", outcome.Status)
	}
	if outcome.CountsAsHealthFailure() {
		t.Fatal("client cancellation must not count as provider health failure")
	}
}

func TestClassifyOutcomeRetriesRejectedCredentials(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		outcome := ClassifyOutcome(status, nil)
		if outcome.Status != ResultUnauthorized {
			t.Fatalf("status %d classified as %s, want unauthorized", status, outcome.Status)
		}
		if outcome.Decision(true) != DecisionRetryNewCred || outcome.Decision(false) != DecisionRetry {
			t.Fatalf("status %d did not select credential/route failover", status)
		}
	}
}
