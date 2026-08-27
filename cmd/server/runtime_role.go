package main

import (
	"fmt"
	"strings"
)

// runtimeRole selects the process shape. all preserves the historical single
// process deployment; the other roles expose only the surfaces needed by
// their deployment topology.
type runtimeRole string

const (
	runtimeRoleAll        runtimeRole = "all"
	runtimeRoleControlAPI runtimeRole = "control-api"
	runtimeRoleGateway    runtimeRole = "gateway"
	runtimeRoleWorker     runtimeRole = "worker"
)

func parseRuntimeRole(args []string) (runtimeRole, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return runtimeRoleAll, nil
	}
	if len(args) > 1 {
		return "", fmt.Errorf("usage: dai [all|control-api|gateway|worker]")
	}
	role := runtimeRole(strings.TrimSpace(args[0]))
	if err := role.Validate(); err != nil {
		return "", err
	}
	return role, nil
}

func (r runtimeRole) Validate() error {
	switch r {
	case runtimeRoleAll, runtimeRoleControlAPI, runtimeRoleGateway, runtimeRoleWorker:
		return nil
	default:
		return fmt.Errorf("unknown runtime role %q; want all, control-api, gateway, or worker", r)
	}
}

func (r runtimeRole) HasControlAPI() bool { return r == runtimeRoleAll || r == runtimeRoleControlAPI }
func (r runtimeRole) HasGateway() bool    { return r == runtimeRoleAll || r == runtimeRoleGateway }
func (r runtimeRole) HasWorkers() bool    { return r == runtimeRoleAll || r == runtimeRoleWorker }
