package gateway

import (
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/libs/go/server"
)

func TestRegisterOpenAPIAddsRunRuntimeSpec(t *testing.T) {
	_, api := server.New(server.Options{Title: "D-AI", Version: "test"})

	RegisterOpenAPI(api)

	doc := api.OpenAPI()
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		t.Fatalf("security schemes not initialized")
	}
	scheme := doc.Components.SecuritySchemes["bearerAuth"]
	if scheme == nil {
		t.Fatalf("bearerAuth scheme not registered")
	}
	if scheme.Type != "http" || scheme.Scheme != "bearer" {
		t.Fatalf("unexpected bearerAuth scheme: %+v", scheme)
	}
	path := doc.Paths["/v1/run"]
	if path == nil || path.Post == nil {
		t.Fatalf("/v1/run POST not registered")
	}
	op := path.Post
	if op.OperationID != "ai-run-runtime" {
		t.Fatalf("operation id = %q, want ai-run-runtime", op.OperationID)
	}
	if len(op.Security) != 1 || op.Security[0]["bearerAuth"] == nil {
		t.Fatalf("operation security missing bearerAuth")
	}
	if op.Method != http.MethodPost {
		t.Fatalf("method = %q, want %q", op.Method, http.MethodPost)
	}

	// /v1/run/images/* no longer exist: chat/image_generation/image_edit are
	// unified onto the single /v1/run operation above, disambiguated by the
	// app key's bound app type rather than the URL path.
	if doc.Paths["/v1/run/images/generations"] != nil {
		t.Fatalf("/v1/run/images/generations should no longer be registered")
	}
	if doc.Paths["/v1/run/images/edits"] != nil {
		t.Fatalf("/v1/run/images/edits should no longer be registered")
	}

	if op.RequestBody == nil || op.RequestBody.Content["application/json"] == nil {
		t.Fatalf("request body schema missing")
	}
	jsonBody := op.RequestBody.Content["application/json"]
	if len(jsonBody.Examples) < 4 {
		t.Fatalf("request body examples missing, got %d", len(jsonBody.Examples))
	}
	if len(jsonBody.Schema.OneOf) != 3 {
		t.Fatalf("request body schema should oneOf chat/image-generation/image-edit, got %d variants", len(jsonBody.Schema.OneOf))
	}
	if op.RequestBody.Content["multipart/form-data"] == nil {
		t.Fatalf("multipart/form-data content missing for image edit uploads")
	}

	imageEditJSONSchema := doc.Components.Schemas.SchemaFromRef(jsonBody.Schema.OneOf[2].Ref)
	if imageEditJSONSchema == nil || imageEditJSONSchema.Properties["images"] == nil {
		t.Fatalf("image edit json schema missing multi-reference images field")
	}
	assertSchemaProperties(t, imageEditJSONSchema, "input", "variables", "n", "images", "mask", "stream", "response_format")
	imageSources := imageEditJSONSchema.Properties["images"]
	if imageSources.Items == nil || imageSources.Items.Properties["image_url"] == nil {
		t.Fatalf("image edit images items must expose image_url")
	}
	assertSchemaProperties(t, imageSources.Items, "image_url")

	imageGenSchema := doc.Components.Schemas.SchemaFromRef(jsonBody.Schema.OneOf[1].Ref)
	if imageGenSchema == nil {
		t.Fatalf("image generation schema missing")
	}
	assertSchemaProperties(t, imageGenSchema, "input", "variables", "n", "stream", "size", "response_format", "background", "output_format")

	imageEditMultipartSchema := doc.Components.Schemas.SchemaFromRef(op.RequestBody.Content["multipart/form-data"].Schema.Ref)
	if imageEditMultipartSchema == nil || imageEditMultipartSchema.Properties["image[]"] == nil {
		t.Fatalf("image edit multipart schema missing repeated image[] field")
	}
	assertSchemaProperties(t, imageEditMultipartSchema, "input", "variables", "n", "image[]", "mask", "stream", "response_format")

	if got := op.Responses["200"].Content["text/event-stream"].Schema.Type; got != huma.TypeString {
		t.Fatalf("stream schema type = %q, want %q", got, huma.TypeString)
	}
	if len(op.Responses["200"].Content["application/json"].Schema.OneOf) != 2 {
		t.Fatalf("200 response schema should oneOf chat/image responses")
	}
	if _, ok := op.Responses["503"]; !ok {
		t.Fatalf("503 response missing")
	}
	if op.Responses["400"].Content["application/json"].Examples["partial-images-unsupported"] == nil {
		t.Fatalf("400 examples missing partial-images-unsupported")
	}
	if op.Responses["401"].Content["application/json"].Examples["expired-key"] == nil {
		t.Fatalf("401 examples missing expired-key")
	}
	if op.Responses["429"].Content["application/json"].Examples["rate-limit"] == nil {
		t.Fatalf("429 examples missing rate-limit")
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
