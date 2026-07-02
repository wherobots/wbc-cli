package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// makeTestJWT builds an unsigned JWT-shaped token with the given payload.
func makeTestJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(encoded) + ".signature"
}

func TestDecodeClaims(t *testing.T) {
	t.Parallel()
	token := makeTestJWT(t, map[string]any{
		"sub":   "user_01ABC",
		"email": "clay@wherobots.com",
		"exp":   1751500000,
	})

	claims := DecodeClaims(token)
	if claims.Sub != "user_01ABC" {
		t.Errorf("Sub = %q", claims.Sub)
	}
	if claims.Email != "clay@wherobots.com" {
		t.Errorf("Email = %q", claims.Email)
	}
	if claims.Exp != 1751500000 {
		t.Errorf("Exp = %d", claims.Exp)
	}
}

func TestDecodeClaimsGarbageReturnsZero(t *testing.T) {
	t.Parallel()
	for _, token := range []string{"", "not-a-jwt", "a.b", "a.!!!.c", "a." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".c"} {
		claims := DecodeClaims(token)
		if claims != (Claims{}) {
			t.Errorf("DecodeClaims(%q) = %+v, want zero", token, claims)
		}
	}
}
