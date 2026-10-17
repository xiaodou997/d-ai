package serving

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/formats"
)

// executeCodexSyncStream waits for the authoritative completed response. It
// preserves function calls and encrypted reasoning items verbatim, meters every
// native event, and never turns a truncated or failed stream into JSON success.
func (s *ExecuteStep) executeCodexSyncStream(dc *deadlineController, req *Request, resp *UpstreamResponse, convert bool) error {
	defer ApplyReportedUsage(req)
	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	var size int64
	for {
		event, readErr := readUpstreamEvent(reader)
		size += int64(len(event.raw))
		if size > syncBodyLimit(req) {
			return &precommitError{cause: errSSELineTooLong, httpStatus: resp.StatusCode, message: "upstream response exceeded the size limit"}
		}
		if len(event.raw) > 0 {
			dc.firstByte()
			dc.chunkReceived()
		}
		if len(event.data) > 0 && !bytes.Equal(bytes.TrimSpace(event.data), []byte("[DONE]")) {
			observeReportedUsage(req, event.data, event.event)
			observeProviderStreamFrame(req, event.data, event.event)
			outcome := formats.InspectStreamOutcome(event.data, event.event)
			if req.ProviderTerminalState == domain.ProviderTerminalFailed || req.ProviderTerminalState == domain.ProviderTerminalIncomplete || req.ProviderTerminalState == domain.ProviderTerminalCancelled {
				// A bare error may be followed by a failure snapshot carrying
				// usage. Preserve that accounting before terminating the attempt.
				failureTerminal := !outcome.BareError && (outcome.State == domain.ProviderTerminalFailed || outcome.State == domain.ProviderTerminalIncomplete || outcome.State == domain.ProviderTerminalCancelled)
				if !failureTerminal && readErr == nil {
					continue
				}
				if failureTerminal || readErr != nil {
					req.BillingReason = "provider_terminal_failure"
					req.UpstreamResponseBody = event.data
					return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode, message: "upstream response did not complete successfully"}
				}
			} else if req.ProviderTerminalState == domain.ProviderTerminalCompleted {
				var completed struct {
					Response json.RawMessage `json:"response"`
				}
				if err := json.Unmarshal(event.data, &completed); err != nil || len(completed.Response) == 0 || bytes.Equal(completed.Response, []byte("null")) {
					return &precommitError{cause: errUpstreamErrorBody, httpStatus: resp.StatusCode, message: "upstream completed without a response snapshot"}
				}
				jsonResponse := &UpstreamResponse{StatusCode: resp.StatusCode, Headers: resp.Headers.Clone(), Body: io.NopCloser(bytes.NewReader(completed.Response))}
				jsonResponse.Headers.Set("Content-Type", "application/json")
				dc.syncBodyPhase()
				if convert {
					return s.executeSyncConvert(dc, req, jsonResponse, req.Envelope.W)
				}
				return s.executeSync(dc, req, jsonResponse, req.Envelope.W)
			}
		}
		if readErr != nil {
			if cause := dc.cause(); cause != nil {
				readErr = cause
			}
			markProviderTerminal(req, domain.ProviderTerminalIncomplete)
			message := "upstream stream ended before a completed response"
			if readErr != io.EOF {
				message = fmt.Sprintf("read upstream response: %s", strings.TrimSpace(readErr.Error()))
			}
			return &precommitError{cause: readErr, httpStatus: resp.StatusCode, message: message}
		}
	}
}
