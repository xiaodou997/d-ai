package gateway

import (
	"net/http"
	"testing"

	"xiaodou/dai/internal/ai/asynctask"
)

func TestResolveTaskType(t *testing.T) {
	tests := []struct {
		name     string
		wireType string
		want     string
		wantCode string
	}{
		{name: "generation", wireType: "images.generation", want: apiImageGenerationTaskType},
		{name: "edit", wireType: "images.edit", want: apiImageEditTaskType},
		{name: "chat", wireType: "chat.completions", want: apiChatCompletionTaskType},
		{name: "required", wantCode: "task_type_required"},
		{name: "unsupported", wireType: "embeddings", wantCode: "unsupported_task_type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTaskType(tt.wireType)
			if tt.wantCode == "" {
				if err != nil || got != tt.want {
					t.Fatalf("resolveTaskType = %q, %v; want %q, nil", got, err, tt.want)
				}
				return
			}
			apiErr := asynctask.AsError(err)
			if apiErr.Status != http.StatusBadRequest || apiErr.Code != tt.wantCode {
				t.Fatalf("error = %+v, want 400 %s", apiErr, tt.wantCode)
			}
		})
	}
}

func TestWireTaskTypeHidesRegistrySurface(t *testing.T) {
	for registryType, want := range map[string]string{
		apiImageGenerationTaskType: imageGenerationWireTaskType,
		apiImageEditTaskType:       imageEditWireTaskType,
		apiChatCompletionTaskType:  chatCompletionWireTaskType,
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

func TestPublicTaskRegistryTypesIncludesChat(t *testing.T) {
	types, err := publicTaskRegistryTypes(chatCompletionWireTaskType)
	if err != nil || len(types) != 1 || types[0] != apiChatCompletionTaskType {
		t.Fatalf("chat registry types = %v, %v", types, err)
	}
}
