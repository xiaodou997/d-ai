package transport

import (
	"context"
	"testing"

	"xiaodou/dai/internal/auth"
	authports "xiaodou/dai/internal/auth/ports"
)

type tenantOperationsAccountReaderStub struct {
	requestedUserID   string
	requestedUserType int
	snapshot          authports.CurrentUserSnapshot
}

func (s *tenantOperationsAccountReaderStub) GetCurrentUserSnapshot(_ context.Context, userID string, userType int) (authports.CurrentUserSnapshot, error) {
	s.requestedUserID = userID
	s.requestedUserType = userType
	return s.snapshot, nil
}

func (*tenantOperationsAccountReaderStub) GetPasswordHash(context.Context, string, int) (string, error) {
	return "", nil
}

func (*tenantOperationsAccountReaderStub) CheckTenantActive(context.Context, string) (bool, error) {
	return true, nil
}

func TestTenantOperationsCurrentUserProjectsOperatorIntoTenantScope(t *testing.T) {
	reader := &tenantOperationsAccountReaderStub{snapshot: authports.CurrentUserSnapshot{
		UserID: "admin-1", Username: "operator", UserType: int(auth.UserTypePlatformAdmin),
		MFAEnabled: true, Status: "active",
	}}
	snapshot, err := queryCurrentUserSnapshot(context.Background(), authModule{AuthAccountReader: reader}, &auth.Claims{
		UserID: "admin-1", UserType: int(auth.UserTypeTenant), TenantID: "tenant-1",
		TenantName: "Tenant One", TenantOperations: true,
		OperatorID: "admin-1", OperatorUserType: int(auth.UserTypePlatformAdmin),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reader.requestedUserID != "admin-1" || reader.requestedUserType != int(auth.UserTypePlatformAdmin) {
		t.Fatalf("operator lookup = (%q, %d)", reader.requestedUserID, reader.requestedUserType)
	}
	if snapshot.userType != int(auth.UserTypeTenant) || snapshot.tenantID != "tenant-1" || snapshot.tenantName != "Tenant One" {
		t.Fatalf("effective tenant snapshot = %#v", snapshot)
	}
	if snapshot.userID != "admin-1" || snapshot.username != "operator" || !snapshot.mfaEnabled {
		t.Fatalf("operator identity snapshot = %#v", snapshot)
	}
}
