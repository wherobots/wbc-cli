package executor

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

func RenderCurl(req *http.Request, rawBody string) string {
	parts := []string{
		"curl",
		"-X", req.Method,
		shellQuote(req.URL.String()),
	}

	headerKeys := make([]string, 0, len(req.Header))
	for key := range req.Header {
		headerKeys = append(headerKeys, key)
	}
	slices.Sort(headerKeys)

	for _, key := range headerKeys {
		values := append([]string(nil), req.Header.Values(key)...)
		slices.Sort(values)
		for _, value := range values {
			parts = append(parts, "-H", shellQuote(fmt.Sprintf("%s: %s", key, sanitizeHeaderValue(key, value))))
		}
	}

	body := strings.TrimSpace(rawBody)
	if body != "" {
		parts = append(parts, "--data-raw", shellQuote(body))
	}

	return strings.Join(parts, " ")
}

func sanitizeHeaderValue(key, value string) string {
	if strings.EqualFold(key, "x-api-key") && value != "" {
		return "$WHEROBOTS_API_KEY"
	}
	return value
}

func shellQuote(input string) string {
	return "'" + strings.ReplaceAll(input, "'", `'"'"'`) + "'"
}
