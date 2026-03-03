package executor

import (
	"net/http"
	"testing"
)

func TestRenderCurlIsDeterministicAndMasksToken(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodPatch, "https://api.example.com/users/42?expand=true", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("x-api-key", "secret-value")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	out := RenderCurl(req, `{"enabled":true}`)
	expected := "curl -X PATCH 'https://api.example.com/users/42?expand=true' -H 'Accept: application/json' -H 'Content-Type: application/json' -H 'X-Api-Key: $WHEROBOTS_API_KEY' --data-raw '{\"enabled\":true}'"
	if out != expected {
		t.Fatalf("RenderCurl() = %q\nwant = %q", out, expected)
	}
}
