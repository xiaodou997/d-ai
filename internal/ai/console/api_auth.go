package console

type apiRole string

const (
	apiRolePlatform apiRole = "platform"
	apiRoleTenant   apiRole = "tenant"
	apiRoleUser     apiRole = "user"
)

func roleForUserType(userType int) apiRole {
	switch userType {
	case 1, 2: // 超管、平台管理员
		return apiRolePlatform
	case 3: // 租户用户
		return apiRoleTenant
	case 4: // 终端用户
		return apiRoleUser
	default:
		return ""
	}
}
