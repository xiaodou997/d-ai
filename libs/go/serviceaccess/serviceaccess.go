// Package serviceaccess defines the Redis snapshot contract shared by URM and
// portal business services.
package serviceaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "urm:service_access:"

var (
	ErrDenied      = errors.New("service access denied")
	ErrUnavailable = errors.New("service access unavailable")
)

type ServiceSnapshot struct {
	ServiceID     string `json:"serviceId"`
	Active        bool   `json:"active"`
	PortalEnabled bool   `json:"portalEnabled"`
}

type SubjectSnapshot struct {
	SubjectType string   `json:"subjectType"`
	SubjectID   string   `json:"subjectId"`
	Mode        string   `json:"mode"`
	ServiceIDs  []string `json:"serviceIds"`
	Version     int64    `json:"version"`
}

func ServiceKey(serviceID string) string { return keyPrefix + "service:" + serviceID }
func GlobalFenceKey() string             { return keyPrefix + "global-fence" }
func ServiceFenceKey(serviceID string) string {
	return keyPrefix + "service-fence:" + serviceID
}
func SubjectKey(subjectType, subjectID string) string {
	return keyPrefix + "subject:" + subjectType + ":" + subjectID
}
func FenceKey(subjectType, subjectID string) string {
	return keyPrefix + "fence:" + subjectType + ":" + subjectID
}

// Checker fails closed when Redis, a required snapshot, or an update fence is
// unavailable. Super administrators skip only the subject-policy lookup.
type Checker struct{ redis *redis.Client }

func NewChecker(client *redis.Client) *Checker { return &Checker{redis: client} }

func (c *Checker) Check(ctx context.Context, userType int, userID, tenantID, expectedClientID, tokenClientID string) error {
	if expectedClientID == "" || tokenClientID != expectedClientID {
		return ErrDenied
	}
	if c == nil || c.redis == nil {
		return ErrUnavailable
	}
	globalFenced, err := c.redis.Exists(ctx, GlobalFenceKey()).Result()
	if err != nil {
		return fmt.Errorf("%w: read global reconciliation fence: %v", ErrUnavailable, err)
	}
	if globalFenced != 0 {
		return ErrUnavailable
	}
	serviceFenced, err := c.redis.Exists(ctx, ServiceFenceKey(expectedClientID)).Result()
	if err != nil {
		return fmt.Errorf("%w: read service update fence: %v", ErrUnavailable, err)
	}
	if serviceFenced != 0 {
		return ErrUnavailable
	}

	var service ServiceSnapshot
	if err := c.readJSON(ctx, ServiceKey(expectedClientID), &service); err != nil {
		return err
	}
	if service.ServiceID != expectedClientID || !service.Active || !service.PortalEnabled {
		return ErrDenied
	}
	if userType == 1 {
		return nil
	}

	subjectType, subjectID := subjectFor(userType, userID, tenantID)
	if subjectType == "" || subjectID == "" {
		return ErrDenied
	}
	fenced, err := c.redis.Exists(ctx, FenceKey(subjectType, subjectID)).Result()
	if err != nil {
		return fmt.Errorf("%w: read update fence: %v", ErrUnavailable, err)
	}
	if fenced != 0 {
		return ErrUnavailable
	}

	var subject SubjectSnapshot
	if err := c.readJSON(ctx, SubjectKey(subjectType, subjectID), &subject); err != nil {
		return err
	}
	if subject.SubjectType != subjectType || subject.SubjectID != subjectID || subject.Version < 1 {
		return ErrUnavailable
	}
	switch subject.Mode {
	case "all":
		if len(subject.ServiceIDs) != 0 {
			return ErrUnavailable
		}
		return nil
	case "selected":
		for _, id := range subject.ServiceIDs {
			if id == expectedClientID {
				return nil
			}
		}
		return ErrDenied
	default:
		return ErrUnavailable
	}
}

func (c *Checker) readJSON(ctx context.Context, key string, target any) error {
	raw, err := c.redis.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrUnavailable
		}
		return fmt.Errorf("%w: read %s: %v", ErrUnavailable, key, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: decode %s: %v", ErrUnavailable, key, err)
	}
	return nil
}

func subjectFor(userType int, userID, tenantID string) (string, string) {
	switch userType {
	case 2:
		return "admin", userID
	case 3, 4:
		return "tenant", tenantID
	default:
		return "", ""
	}
}
