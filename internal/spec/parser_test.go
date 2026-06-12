package spec

import (
	"testing"
)

func TestParseExcludeFromCLIExtension(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "openapi": "3.0.3",
  "info": { "title": "x", "version": "1" },
  "paths": {
    "/public": {
      "get": {
        "operationId": "getPublic",
        "responses": { "200": { "description": "ok" } }
      }
    },
    "/secret": {
      "post": {
        "operationId": "createSecret",
        "x-exclude-from-cli": true,
        "responses": { "200": { "description": "ok" } }
      }
    },
    "/also-secret": {
      "delete": {
        "operationId": "deleteSecret",
        "x-exclude-from-cli": true,
        "responses": { "200": { "description": "ok" } }
      }
    }
  }
}`)

	parsed, err := Parse(raw, "")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Operations) != 3 {
		t.Fatalf("want 3 operations, got %d", len(parsed.Operations))
	}

	byID := make(map[string]*Operation, len(parsed.Operations))
	for _, op := range parsed.Operations {
		byID[op.OperationID] = op
	}

	if byID["getPublic"].Excluded {
		t.Error("getPublic should not be excluded")
	}
	if !byID["createSecret"].Excluded {
		t.Error("createSecret should be excluded (x-exclude-from-cli: true)")
	}
	if !byID["deleteSecret"].Excluded {
		t.Error("deleteSecret should be excluded (x-exclude-from-cli: true)")
	}
}

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

// TestParseResolvesComposedObjectBodyFields covers the WBC-43 bug: FastAPI/Pydantic
// emits optional object fields as `anyOf: [{$ref -> object}, {type: "null"}]` and
// required object fields as a bare `$ref`. Both must resolve to "object" (and arrays
// to "array") so the command builder generates JSON-object flags rather than treating
// them as plain strings.
func TestParseResolvesComposedObjectBodyFields(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "openapi": "3.1.0",
  "info": { "title": "x", "version": "1" },
  "servers": [{ "url": "https://api.example.com" }],
  "paths": {
    "/runs": {
      "post": {
        "operationId": "createRun",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["runPython"],
                "properties": {
                  "name": { "type": "string" },
                  "runPython": { "anyOf": [ { "$ref": "#/components/schemas/RunPython" }, { "type": "null" } ] },
                  "runJar": { "$ref": "#/components/schemas/RunJar" },
                  "tags": { "anyOf": [ { "type": "array", "items": { "type": "string" } }, { "type": "null" } ] }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "ok" } }
      }
    }
  },
  "components": {
    "schemas": {
      "RunPython": {
        "type": "object",
        "required": ["uri"],
        "properties": {
          "uri": { "type": "string" },
          "args": { "type": "array", "items": { "type": "string" } }
        }
      },
      "RunJar": {
        "type": "object",
        "required": ["uri"],
        "properties": { "uri": { "type": "string" } }
      }
    }
  }
}`)

	parsed, err := Parse(raw, "")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(parsed.Operations) != 1 {
		t.Fatalf("operations = %d, want 1", len(parsed.Operations))
	}

	byField := make(map[string]BodyField)
	for _, field := range parsed.Operations[0].RequestBody.Fields {
		byField[field.Name] = field
	}

	cases := map[string]string{
		"name":      "string",
		"runPython": "object",
		"runJar":    "object",
		"tags":      "array",
	}
	for name, wantType := range cases {
		field, ok := byField[name]
		if !ok {
			t.Fatalf("field %q missing from parsed body fields", name)
		}
		if field.Type != wantType {
			t.Errorf("field %q type = %q, want %q", name, field.Type, wantType)
		}
	}
}

// TestParseScalarUnionPrefersFirstConcreteType guards against over-eager
// composition resolution: a `str | int` union must resolve to its first
// concrete alternative ("string"), not skip past it to a stricter type. The
// resolver must only treat "null" (and unresolvable members) as skippable.
func TestParseScalarUnionPrefersFirstConcreteType(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "openapi": "3.1.0",
  "info": { "title": "x", "version": "1" },
  "paths": {
    "/things": {
      "post": {
        "operationId": "createThing",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "mode": { "anyOf": [ { "type": "string" }, { "type": "integer" } ] }
                }
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

	var mode *BodyField
	for i := range parsed.Operations[0].RequestBody.Fields {
		if parsed.Operations[0].RequestBody.Fields[i].Name == "mode" {
			mode = &parsed.Operations[0].RequestBody.Fields[i]
		}
	}
	if mode == nil {
		t.Fatal("field \"mode\" missing from parsed body fields")
	}
	if mode.Type != "string" {
		t.Errorf("field \"mode\" type = %q, want \"string\"", mode.Type)
	}
}
