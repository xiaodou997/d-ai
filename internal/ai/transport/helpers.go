package transport

import (
	"fmt"

	"github.com/google/uuid"
)

func parseTransportUUID(value string) (uuid.UUID, error) {
	if len(value) != 32 && len(value) != 36 {
		return uuid.Nil, fmt.Errorf("invalid UUID length: %d", len(value))
	}
	return uuid.Parse(value)
}
