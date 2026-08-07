package gateway

import (
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/libs/go/server"
)

func TestRegisterOpenAPIRemovesRunRuntimeSpec(t *testing.T) {
	_, api := server.New(server.Options{Title: "D-AI", Version: "test"})

	RegisterOpenAPI(api)

	doc := api.OpenAPI()
	for _, path := range []string{"/v1/run", "/v1/run/images/generations", "/v1/run/images/edits"} {
		if doc.Paths[path] != nil {
			t.Fatalf("retired application runtime path %s is still registered", path)
		}
	}
}

func TestRegisterOpenAPIAddsAsyncTaskSpec(t *testing.T) {
	_, api := server.New(server.Options{Title: "D-AI", Version: "test"})

	RegisterOpenAPI(api)

	doc := api.OpenAPI()
	scheme := doc.Components.SecuritySchemes["taskBearerAuth"]
	if scheme == nil || scheme.Type != "http" || scheme.Scheme != "bearer" {
		t.Fatalf("taskBearerAuth scheme = %+v", scheme)
	}
	collection := doc.Paths["/v1/tasks"]
	if collection == nil || collection.Post == nil {
		t.Fatal("POST /v1/tasks not registered")
	}
	if collection.Get == nil || collection.Get.OperationID != "ai-list-tasks" {
		t.Fatal("GET /v1/tasks list operation not registered")
	}
	create := collection.Post
	if create.OperationID != "ai-create-task" || len(create.Security) != 1 || create.Security[0]["taskBearerAuth"] == nil {
		t.Fatalf("create operation = %+v", create)
	}
	if create.RequestBody == nil || create.RequestBody.Content["application/json"] == nil || create.RequestBody.Content["multipart/form-data"] == nil {
		t.Fatal("task JSON/multipart request schemas missing")
	}
	jsonSchema := doc.Components.Schemas.SchemaFromRef(create.RequestBody.Content["application/json"].Schema.Ref)
	multipartSchema := doc.Components.Schemas.SchemaFromRef(create.RequestBody.Content["multipart/form-data"].Schema.Ref)
	if jsonSchema == nil || jsonSchema.Properties["webhook_url"] == nil ||
		multipartSchema == nil || multipartSchema.Properties["webhook_url"] == nil {
		t.Fatal("webhook_url is not documented for both task submission transports")
	}
	if !schemaEnumContains(jsonSchema.Properties["type"], "chat.completions") {
		t.Fatal("chat.completions is missing from the task submission type enum")
	}
	if !strings.Contains(create.Description, "source=D-AI") ||
		!strings.Contains(create.Description, "GET /v1/tasks/{id}") ||
		!strings.Contains(create.Description, "stream=false") {
		t.Fatal("minimal webhook notification contract is not documented")
	}
	if create.Responses["202"] == nil {
		t.Fatal("task 202 response missing")
	}
	foundIdempotency := false
	for _, parameter := range create.Parameters {
		if parameter.In == "header" && parameter.Name == "Idempotency-Key" {
			foundIdempotency = true
		}
	}
	if !foundIdempotency {
		t.Fatal("Idempotency-Key header not documented")
	}

	item := doc.Paths["/v1/tasks/{taskID}"]
	if item == nil || item.Get == nil || item.Get.OperationID != "ai-get-task" {
		t.Fatalf("GET /v1/tasks/{taskID} operation = %+v", item)
	}
	cancel := doc.Paths["/v1/tasks/{taskID}/cancel"]
	if cancel == nil || cancel.Post == nil || cancel.Post.OperationID != "ai-cancel-task" {
		t.Fatalf("POST task cancel operation = %+v", cancel)
	}
}

func schemaEnumContains(schema *huma.Schema, want string) bool {
	if schema == nil {
		return false
	}
	for _, value := range schema.Enum {
		if value == want {
			return true
		}
	}
	return false
}

func assertSchemaProperties(t *testing.T, schema *huma.Schema, expected ...string) {
	t.Helper()
	propertyCount := len(schema.Properties)
	if schema.Properties["$schema"] != nil {
		propertyCount--
	}
	if propertyCount != len(expected) {
		t.Fatalf("schema properties = %v, want exactly %v", schema.Properties, expected)
	}
	for _, name := range expected {
		if schema.Properties[name] == nil {
			t.Fatalf("schema property %q missing", name)
		}
	}
}
