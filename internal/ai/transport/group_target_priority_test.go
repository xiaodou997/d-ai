package transport

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/commercial"
	"xiaodou/dai/internal/auth"
	"xiaodou/dai/libs/go/server"
)

type priorityTargetManager struct {
	CommercialGroupTargetManager
	input commercial.GroupTargetWrite
	item  commercial.GroupTargetDetail
	calls int
}

func (s *priorityTargetManager) save(input commercial.GroupTargetWrite) commercial.GroupTarget {
	s.input = input
	s.calls++
	if input.Priority != nil {
		s.item.Priority = *input.Priority
	}
	return s.item.GroupTarget
}

func (s *priorityTargetManager) AddGroupTarget(_ context.Context, _ commercial.TenantGroupScope, input commercial.GroupTargetWrite) (commercial.GroupTarget, error) {
	return s.save(input), nil
}

func (s *priorityTargetManager) UpdateGroupTarget(_ context.Context, _ commercial.TenantGroupScope, _ string, input commercial.GroupTargetWrite) (commercial.GroupTarget, error) {
	return s.save(input), nil
}

func (s *priorityTargetManager) ReplaceGroupTargets(_ context.Context, _ commercial.TenantGroupScope, input commercial.GroupTargetBatchWrite) (commercial.GroupTargetBatchResult, error) {
	return commercial.GroupTargetBatchResult{RoutePolicyVersion: input.ExpectedVersion + 1, Targets: []commercial.GroupTarget{s.save(input.Targets[0])}}, nil
}

func (s *priorityTargetManager) GetGroupTargetDetail(context.Context, commercial.TenantGroupScope, string) (commercial.GroupTargetDetail, error) {
	return s.item, nil
}

func (s *priorityTargetManager) ListGroupTargetDetails(context.Context, commercial.TenantGroupScope) ([]commercial.GroupTargetDetail, error) {
	return []commercial.GroupTargetDetail{s.item}, nil
}

func TestGroupTargetPriorityHTTPContract(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		for _, value := range []string{"", "0", "10", "2147483647", "-1", "1.5", "2147483648"} {
			t.Run(method+"/priority="+value, func(t *testing.T) {
				manager := &priorityTargetManager{item: commercial.GroupTargetDetail{GroupTarget: commercial.GroupTarget{
					ID: "binding-1", GroupID: "group-1", TargetKind: commercial.TargetKindDirectUpstream,
					TargetID: "account-1", Priority: 100, Status: commercial.StatusActive,
				}}}
				router, api := server.New(server.Options{Title: "test", Version: "test"})
				registerGroups(api, TenantGroupManagementHTTPDeps{GroupTargets: manager})
				handler := withCommercialClaims(router, &auth.Claims{TenantID: "tenant-1"})
				path := "/api/v1/tenants/me/groups/group-1/targets"
				body := `{"status":"active"`
				if method == http.MethodPatch {
					path += "/binding-1"
				} else {
					body += `,"account_id":"account-1"`
				}
				if value != "" {
					body += `,"priority":` + value
				}
				body += "}"
				if method == http.MethodPut {
					body = `{"expected_version":1,"targets":[` + body + `]}`
				}
				response := performCommercialRequest(handler, method, path, body)
				if value == "-1" || value == "1.5" || value == "2147483648" {
					requireCommercialStatus(t, response, http.StatusUnprocessableEntity)
					if manager.calls != 0 {
						t.Fatal("invalid priority reached manager")
					}
					return
				}
				requireCommercialStatus(t, response, http.StatusOK)
				if value != "" && (manager.input.Priority == nil || fmt.Sprint(*manager.input.Priority) != value) {
					t.Fatalf("priority %s was not passed to manager: %+v", value, manager.input)
				}
				if value == "" && method != http.MethodPatch && manager.input.Priority != nil {
					t.Fatal("omitted priority must use the repository default")
				}
			})
		}
	}
}

func TestGroupTargetListDoesNotExposeRuntimeHealth(t *testing.T) {
	manager := &priorityTargetManager{item: commercial.GroupTargetDetail{
		GroupTarget: commercial.GroupTarget{ID: "binding-1", GroupID: "group-1", TargetKind: commercial.TargetKindDirectUpstream, TargetID: "account-1", Priority: 10, Status: commercial.StatusActive},
		EndpointIDs: []string{"endpoint-1"},
	}}
	router, api := server.New(server.Options{Title: "test", Version: "test"})
	registerGroups(api, TenantGroupManagementHTTPDeps{Groups: &commercialGroupCatalogStub{}, GroupTargets: manager})
	handler := withCommercialClaims(router, &auth.Claims{TenantID: "tenant-1"})
	response := performCommercialRequest(handler, http.MethodGet, "/api/v1/tenants/me/groups/group-1/targets", "")
	requireCommercialStatus(t, response, http.StatusOK)
	var body struct {
		Items []groupTargetDTO `json:"items"`
	}
	decodeCommercialResponse(t, response, &body)
	if strings.Contains(response.Body.String(), "health_state") {
		t.Fatal("tenant response leaked runtime health")
	}
	if len(body.Items) != 1 || body.Items[0].Priority != 10 {
		t.Fatalf("target health/priority = %+v", body.Items)
	}
}
