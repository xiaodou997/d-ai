package gateway

import (
	"net/http"
	"testing"

	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/asynctask"
	coreidentity "xiaodou/dai/internal/ai/core/identity"
)

func TestResolveTaskTypeForAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		wireType string
		want     string
		wantCode string
	}{
		{name: "generation", wireType: "images.generation", want: "api.images.generation"},
		{name: "edit", wireType: "images.edit", want: "api.images.edit"},
		{name: "chat", wireType: "chat.completions", want: "api.chat.completions"},
		{name: "required", wantCode: "task_type_required"},
		{name: "unsupported", wireType: "embeddings", wantCode: "unsupported_task_type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTaskType(coreidentity.AuthMethodAPIKey, tt.wireType, "")
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("resolveTaskType: %v", err)
				}
				if got != tt.want {
					t.Fatalf("task type = %q, want %q", got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("resolveTaskType returned %q without an error", got)
			}
			apiErr := asynctask.AsError(err)
			if apiErr.Status != http.StatusBadRequest || apiErr.Code != tt.wantCode {
				t.Fatalf("error = %+v, want 400 %s", apiErr, tt.wantCode)
			}
		})
	}
}

func TestResolveTaskTypeForAppKey(t *testing.T) {
	tests := []struct {
		name     string
		appType  application.AppType
		wireType string
		want     string
		wantCode string
	}{
		{name: "generation inferred", appType: application.AppTypeImageGenerationAgent, want: "app.images.generation"},
		{name: "generation asserted", appType: application.AppTypeImageGenerationAgent, wireType: "images.generation", want: "app.images.generation"},
		{name: "edit inferred", appType: application.AppTypeImageEditAgent, want: "app.images.edit"},
		{name: "edit asserted", appType: application.AppTypeImageEditAgent, wireType: "images.edit", want: "app.images.edit"},
		{name: "mismatch", appType: application.AppTypeImageEditAgent, wireType: "images.generation", wantCode: "task_type_mismatch"},
		{name: "chat inferred", appType: application.AppTypeChatAgent, want: "app.chat.completions"},
		{name: "chat asserted", appType: application.AppTypeChatAgent, wireType: "chat.completions", want: "app.chat.completions"},
		{name: "chat mismatch", appType: application.AppTypeChatAgent, wireType: "images.generation", wantCode: "task_type_mismatch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTaskType(coreidentity.AuthMethodInvokeKey, tt.wireType, tt.appType)
			if tt.wantCode == "" {
				if err != nil || got != tt.want {
					t.Fatalf("resolveTaskType = %q, %v; want %q, nil", got, err, tt.want)
				}
				return
			}
			if err == nil || asynctask.AsError(err).Code != tt.wantCode {
				t.Fatalf("resolveTaskType error = %v, want %s", err, tt.wantCode)
			}
		})
	}
}

func TestWireTaskTypeHidesRegistrySurface(t *testing.T) {
	for registryType, want := range map[string]string{
		"api.images.generation": "images.generation",
		"api.images.edit":       "images.edit",
		"app.images.generation": "images.generation",
		"app.images.edit":       "images.edit",
		"api.chat.completions":  "chat.completions",
		"app.chat.completions":  "chat.completions",
	} {
		got, ok := wireTaskType(registryType)
		if !ok || got != want {
			t.Fatalf("wireTaskType(%q) = %q, %v; want %q, true", registryType, got, ok, want)
		}
	}
	if _, ok := wireTaskType("console.images.generation"); ok {
		t.Fatal("console registry type was exposed by the public task API")
	}
}

func TestPublicTaskRegistryTypesIncludesChatSurfaces(t *testing.T) {
	types, err := publicTaskRegistryTypes(chatCompletionWireTaskType)
	if err != nil {
		t.Fatalf("publicTaskRegistryTypes: %v", err)
	}
	want := []string{apiChatCompletionTaskType, appChatCompletionTaskType}
	if len(types) != len(want) || types[0] != want[0] || types[1] != want[1] {
		t.Fatalf("chat registry types = %v, want %v", types, want)
	}

	all, err := publicTaskRegistryTypes("")
	if err != nil {
		t.Fatalf("publicTaskRegistryTypes(all): %v", err)
	}
	for _, taskType := range want {
		found := false
		for _, candidate := range all {
			if candidate == taskType {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("all public task types missing %q: %v", taskType, all)
		}
	}
}
