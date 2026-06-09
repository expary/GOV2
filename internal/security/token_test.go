package security

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTokenSignAndVerify(t *testing.T) {
	manager := NewTokenManager("secret", time.Hour, "gov2-test")
	token, claims, err := manager.Sign(1, "admin", []uint64{1})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	verified, err := manager.Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.Subject != claims.Subject || verified.Username != "admin" {
		t.Fatalf("unexpected claims: %+v", verified)
	}
}

func TestTokenVerifyRejectsTamperedPayload(t *testing.T) {
	manager := NewTokenManager("secret", time.Hour, "gov2-test")
	token, _, err := manager.Sign(1, "admin", []uint64{1})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	parts := strings.Split(token, ".")
	parts[1] = encodeJSON(t, Claims{
		Subject:   2,
		Username:  "attacker",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		Issuer:    "gov2-test",
	})

	if _, err := manager.Verify(strings.Join(parts, ".")); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected tampered token to be invalid, got %v", err)
	}
}

func TestTokenVerifyRejectsWrongIssuer(t *testing.T) {
	manager := NewTokenManager("secret", time.Hour, "gov2-test")
	token := signedToken(t, manager, tokenHeader{Algorithm: "HS256", Type: "GOV2"}, Claims{
		Subject:   1,
		Username:  "admin",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		Issuer:    "other-issuer",
	})

	if _, err := manager.Verify(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected wrong issuer to be invalid, got %v", err)
	}
}

func TestTokenVerifyRejectsInvalidHeader(t *testing.T) {
	manager := NewTokenManager("secret", time.Hour, "gov2-test")
	for _, header := range []tokenHeader{
		{Algorithm: "none", Type: "GOV2"},
		{Algorithm: "HS256", Type: "JWT"},
	} {
		t.Run(header.Algorithm+" "+header.Type, func(t *testing.T) {
			token := signedToken(t, manager, header, Claims{
				Subject:   1,
				Username:  "admin",
				IssuedAt:  time.Now().Unix(),
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Issuer:    "gov2-test",
			})
			if _, err := manager.Verify(token); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("expected invalid header to be rejected, got %v", err)
			}
		})
	}
}

func TestTokenVerifyRejectsExpiredToken(t *testing.T) {
	manager := NewTokenManager("secret", time.Hour, "gov2-test")
	token := signedToken(t, manager, tokenHeader{Algorithm: "HS256", Type: "GOV2"}, Claims{
		Subject:   1,
		Username:  "admin",
		IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-time.Minute).Unix(),
		Issuer:    "gov2-test",
	})

	if _, err := manager.Verify(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected expired token, got %v", err)
	}
}

func TestTokenVerifyRejectsMissingRequiredClaims(t *testing.T) {
	manager := NewTokenManager("secret", time.Hour, "gov2-test")
	token := signedToken(t, manager, tokenHeader{Algorithm: "HS256", Type: "GOV2"}, Claims{
		Username:  "admin",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		Issuer:    "gov2-test",
	})

	if _, err := manager.Verify(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected missing subject to be invalid, got %v", err)
	}
}

func TestBearerTokenParsing(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		want          string
	}{
		{name: "standard", authorization: "Bearer token-value", want: "token-value"},
		{name: "case insensitive", authorization: "bearer token-value", want: "token-value"},
		{name: "trim token", authorization: "Bearer  token-value  ", want: "token-value"},
		{name: "missing token", authorization: "Bearer ", want: ""},
		{name: "wrong scheme", authorization: "Basic token-value", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BearerToken(tt.authorization); got != tt.want {
				t.Fatalf("BearerToken(%q) = %q, want %q", tt.authorization, got, tt.want)
			}
		})
	}
}

func signedToken(t *testing.T, manager *TokenManager, header tokenHeader, claims Claims) string {
	t.Helper()

	head := encodeJSON(t, header)
	body := encodeJSON(t, claims)
	return head + "." + body + "." + manager.signature(head+"."+body)
}

func encodeJSON(t *testing.T, value any) string {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
