package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// Claims are the JWT payload fields the CLI displays. The token is decoded
// WITHOUT signature verification — display and expiry hints only; the
// backend is the validator.
type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
}

// DecodeClaims extracts display claims from a JWT, returning the zero value
// on any malformed input.
func DecodeClaims(token string) Claims {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}
	}
	return claims
}
