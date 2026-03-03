package commands

import (
	"regexp"
	"strings"
	"unicode"

	"wherobots/cli/internal/spec"
)

var (
	pathParamToken = regexp.MustCompile(`^\{[^{}]+\}$`)
	wordSplitter   = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	camelBoundary  = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

func PathToResourceSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if pathParamToken.MatchString(part) {
			continue
		}
		if normalized := normalizeToken(part); normalized != "" {
			segments = append(segments, normalized)
		}
	}
	return segments
}

func ChooseVerb(op *spec.Operation) string {
	if op == nil {
		return "call"
	}
	if fromID := operationIDVerb(op.OperationID); fromID != "" {
		return fromID
	}

	switch strings.ToUpper(op.Method) {
	case "GET":
		if len(op.PathParamOrder) > 0 {
			return "get"
		}
		return "list"
	case "POST":
		return "create"
	case "PUT":
		return "replace"
	case "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return strings.ToLower(op.Method)
	}
}

func operationIDVerb(operationID string) string {
	if operationID == "" {
		return ""
	}
	operationID = camelBoundary.ReplaceAllString(operationID, `$1-$2`)
	return normalizeToken(operationID)
}

func normalizeToken(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	input = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, input)

	parts := wordSplitter.Split(strings.ToLower(input), -1)
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, "-")
}
