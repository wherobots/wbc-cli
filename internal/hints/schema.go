package hints

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"wherobots/cli/internal/spec"
)

func BuildBodyTemplate(op *spec.Operation) string {
	if op == nil || op.RequestBody == nil {
		return "{}"
	}
	if len(op.RequestBody.Fields) == 0 {
		return sampleValueForType(op.RequestBody.SchemaType)
	}

	body := "{}"
	used := 0
	for _, field := range op.RequestBody.Fields {
		if !field.Required {
			continue
		}
		switch field.Type {
		case "integer", "number":
			body, _ = sjson.Set(body, field.Name, 0)
		case "boolean":
			body, _ = sjson.Set(body, field.Name, false)
		case "array":
			body, _ = sjson.SetRaw(body, field.Name, "[]")
		case "object":
			body, _ = sjson.SetRaw(body, field.Name, "{}")
		default:
			body, _ = sjson.Set(body, field.Name, "string")
		}
		used++
	}
	if used == 0 {
		// No required fields; include one optional field per operation for discoverability.
		for _, field := range op.RequestBody.Fields {
			switch field.Type {
			case "integer", "number":
				body, _ = sjson.Set(body, field.Name, 0)
			case "boolean":
				body, _ = sjson.Set(body, field.Name, false)
			case "array":
				body, _ = sjson.SetRaw(body, field.Name, "[]")
			case "object":
				body, _ = sjson.SetRaw(body, field.Name, "{}")
			default:
				body, _ = sjson.Set(body, field.Name, "string")
			}
			break
		}
	}

	if !gjson.Valid(body) {
		return "{}"
	}
	return body
}

func RequiredPathParams(op *spec.Operation) string {
	if op == nil || len(op.PathParamOrder) == 0 {
		return "[]"
	}
	return "[" + strings.Join(op.PathParamOrder, ", ") + "]"
}

func RequiredBodyParams(op *spec.Operation) string {
	if op == nil || op.RequestBody == nil || len(op.RequestBody.Fields) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(op.RequestBody.Fields))
	for _, field := range op.RequestBody.Fields {
		if !field.Required {
			continue
		}
		parts = append(parts, field.Name)
	}
	if len(parts) == 0 {
		return "[]"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func ExpectedTypeSummary(op *spec.Operation) string {
	if op == nil {
		return "none"
	}

	path := make([]string, 0, len(op.PathParams))
	for _, param := range op.PathParams {
		path = append(path, fmt.Sprintf("%s:%s", param.Name, defaultType(param.Type)))
	}

	query := make([]string, 0, len(op.QueryParams))
	for _, param := range op.QueryParams {
		query = append(query, fmt.Sprintf("%s:%s", param.Name, defaultType(param.Type)))
	}

	body := make([]string, 0)
	if op.RequestBody != nil {
		for _, field := range op.RequestBody.Fields {
			suffix := ""
			if field.Required {
				suffix = " (required)"
			}
			body = append(body, fmt.Sprintf("%s:%s%s", field.Name, defaultType(field.Type), suffix))
		}
		if len(body) == 0 {
			body = append(body, fmt.Sprintf("body:%s", defaultType(op.RequestBody.SchemaType)))
		}
	}

	parts := make([]string, 0, 3)
	if len(path) > 0 {
		parts = append(parts, "Path "+formatList(path))
	}
	if len(query) > 0 {
		parts = append(parts, "Query "+formatList(query))
	}
	if len(body) > 0 {
		parts = append(parts, "Body "+formatList(body))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "; ")
}

func sampleValueForType(kind string) string {
	switch kind {
	case "integer", "number":
		return "0"
	case "boolean":
		return "true"
	case "array":
		return "[]"
	case "object":
		return "{}"
	default:
		return `"string"`
	}
}

func defaultType(kind string) string {
	if strings.TrimSpace(kind) == "" {
		return "string"
	}
	return kind
}

func formatList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	return "[" + strings.Join(values, ", ") + "]"
}
