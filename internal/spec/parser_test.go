package spec

import "testing"

func TestParseExtractsOperationsAndSchema(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "openapi": "3.0.3",
  "info": { "title": "x", "version": "1" },
  "servers": [{ "url": "https://api.example.com" }],
  "paths": {
    "/users/{id}/settings": {
      "patch": {
        "operationId": "updateUserSettings",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } },
          { "name": "verbose", "in": "query", "required": true, "schema": { "type": "boolean" } }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["enabled"],
                "properties": { "enabled": { "type": "boolean" } }
              }
            }
          }
        },
        "responses": { "200": { "description": "ok" } }
      }
    }
  }
}`)

	parsed, err := Parse(raw, "")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.BaseURL != "https://api.example.com" {
		t.Fatalf("BaseURL = %s, want https://api.example.com", parsed.BaseURL)
	}
	if len(parsed.Operations) != 1 {
		t.Fatalf("operations = %d, want 1", len(parsed.Operations))
	}

	op := parsed.Operations[0]
	if op.Method != "PATCH" {
		t.Fatalf("method = %s, want PATCH", op.Method)
	}
	if len(op.PathParamOrder) != 1 || op.PathParamOrder[0] != "id" {
		t.Fatalf("path param order = %v, want [id]", op.PathParamOrder)
	}
	if op.RequestBody == nil || !op.RequestBody.Required {
		t.Fatalf("request body should be required")
	}
	if op.RequestBody.SchemaType != "object" {
		t.Fatalf("schema type = %s, want object", op.RequestBody.SchemaType)
	}
	if len(op.RequestBody.Fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(op.RequestBody.Fields))
	}
	if op.RequestBody.Fields[0].Name != "enabled" || op.RequestBody.Fields[0].Type != "boolean" || !op.RequestBody.Fields[0].Required {
		t.Fatalf("field = %+v, want enabled:boolean required", op.RequestBody.Fields[0])
	}
}
