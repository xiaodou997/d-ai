package transport

import (
	"encoding/json"
	"net/http"
	"time"
)

// writeJSON 写出 chi 原生回调端点的标准 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// userTypeDisplayName 返回用户类型的中文展示名。
func userTypeDisplayName(userType int) string {
	switch userType {
	case 1:
		return "超级管理员"
	case 2:
		return "平台管理员"
	case 3:
		return "租户用户"
	case 4:
		return "终端用户"
	default:
		return "未知"
	}
}

func nowUTC() time.Time { return time.Now().UTC() }

type successOutput struct {
	Body struct {
		Success bool `json:"success"`
	}
}

func okSuccess() *successOutput {
	out := &successOutput{}
	out.Body.Success = true
	return out
}
