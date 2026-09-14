package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Key is execution identity; RouteID remains the durable group-target UUID.
func (c *RouteCandidate) Key() string {
	if c == nil {
		return ""
	}
	if c.CandidateID != "" {
		return c.CandidateID
	}
	return c.RouteID
}
func CandidateIdentity(groupTarget, endpoint, model, operation string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{groupTarget, endpoint, model, operation}, "\x00")))
	return hex.EncodeToString(sum[:])
}
func (c *RouteCandidate) OperationKey() string {
	if c.Operation != "" {
		return c.Operation
	}
	return string(c.Protocol)
}
func (c *RouteCandidate) StatisticsKey(stream bool) string {
	mode := "sync"
	if stream {
		mode = "stream"
	}
	physical := c.EndpointID
	if c.IsPoolRoute() {
		physical = "pool:" + c.PoolID
		if c.CredentialID != "" {
			physical = "credential:" + c.CredentialID
		}
	}
	return CandidateIdentity(physical, c.EffectiveUpstreamModel(), c.OperationKey(), mode)
}
func (c *RouteCandidate) CapacityKey() string {
	if c.IsPoolRoute() {
		if c.CredentialID != "" {
			return "credential:" + c.CredentialID
		}
		return "pool:" + c.PoolID
	}
	return "account:" + c.EffectiveAccountID()
}
