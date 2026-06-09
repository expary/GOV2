package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	PasswordMinLength  = 8
	passwordAlgorithm  = "pbkdf2_sha256"
	passwordIterations = 120000
	passwordSaltBytes  = 16
	passwordKeyBytes   = 32
)

func ValidatePasswordPolicy(password string) error {
	if len([]rune(password)) < PasswordMinLength {
		return fmt.Errorf("Password must be at least %d characters", PasswordMinLength)
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("Password must contain a non-whitespace character")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is required")
	}

	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	key := pbkdf2SHA256([]byte(password), salt, passwordIterations, passwordKeyBytes)
	return fmt.Sprintf("%s$%d$%s$%s",
		passwordAlgorithm,
		passwordIterations,
		base64.RawURLEncoding.EncodeToString(salt),
		base64.RawURLEncoding.EncodeToString(key),
	), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordAlgorithm {
		return false
	}

	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}

	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}

	actual := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hashLen := sha256.Size
	blocks := (keyLen + hashLen - 1) / hashLen
	output := make([]byte, 0, blocks*hashLen)

	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{
			byte(block >> 24),
			byte(block >> 16),
			byte(block >> 8),
			byte(block),
		})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)

		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		output = append(output, t...)
	}

	return output[:keyLen]
}
