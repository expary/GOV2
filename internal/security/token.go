package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

type TokenManager struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

type Claims struct {
	Subject   uint64   `json:"sub"`
	Username  string   `json:"username"`
	RoleIDs   []uint64 `json:"role_ids"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	Issuer    string   `json:"iss"`
}

type tokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

func NewTokenManager(secret string, ttl time.Duration, issuer string) *TokenManager {
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	if issuer == "" {
		issuer = "gov2"
	}
	return &TokenManager{
		secret: []byte(secret),
		ttl:    ttl,
		issuer: issuer,
	}
}

func (m *TokenManager) Sign(subject uint64, username string, roleIDs []uint64) (string, Claims, error) {
	now := time.Now()
	claims := Claims{
		Subject:   subject,
		Username:  username,
		RoleIDs:   append([]uint64(nil), roleIDs...),
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(m.ttl).Unix(),
		Issuer:    m.issuer,
	}

	header := tokenHeader{
		Algorithm: "HS256",
		Type:      "GOV2",
	}
	headerData, err := json.Marshal(header)
	if err != nil {
		return "", Claims{}, err
	}
	claimsData, err := json.Marshal(claims)
	if err != nil {
		return "", Claims{}, err
	}

	head := base64.RawURLEncoding.EncodeToString(headerData)
	body := base64.RawURLEncoding.EncodeToString(claimsData)
	signature := m.signature(head + "." + body)
	return head + "." + body + "." + signature, claims, nil
}

func (m *TokenManager) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expected := m.signature(signingInput)
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return Claims{}, ErrInvalidToken
	}

	headerData, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var header tokenHeader
	if err := json.Unmarshal(headerData, &header); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if header.Algorithm != "HS256" || header.Type != "GOV2" {
		return Claims{}, ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if claims.Issuer != m.issuer {
		return Claims{}, ErrInvalidToken
	}
	if claims.Subject == 0 || strings.TrimSpace(claims.Username) == "" || claims.IssuedAt <= 0 || claims.ExpiresAt <= 0 {
		return Claims{}, ErrInvalidToken
	}
	if claims.ExpiresAt <= time.Now().Unix() {
		return Claims{}, ErrExpiredToken
	}
	return claims, nil
}

func (m *TokenManager) signature(input string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func BearerToken(authorization string) string {
	prefix := "Bearer "
	if len(authorization) <= len(prefix) || !strings.EqualFold(authorization[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(authorization[len(prefix):])
}

func SubjectString(id uint64) string {
	return strconv.FormatUint(id, 10)
}

func ParseSubject(value string) (uint64, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid subject: %w", err)
	}
	return id, nil
}
