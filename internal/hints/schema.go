package hints

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"wherobots/cli/internal/spec"
)

func BuildBodyTemplate(op *spec.Operation) string {
	if op == nil || op.RequestBody == nil || len(op.RequestBody.RequiredFields) == 0 {
		return "{}"
	}

	body := "{}"
	for _, field := range op.RequestBody.RequiredFields {
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
	if op == nil || op.RequestBody == nil || len(op.RequestBody.RequiredFields) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(op.RequestBody.RequiredFields))
	for _, field := range op.RequestBody.RequiredFields {
		parts = append(parts, field.Name)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
