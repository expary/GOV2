package security

import "testing"

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("admin123")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !VerifyPassword("admin123", hash) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrong", hash) {
		t.Fatal("expected wrong password to fail")
	}
}

func TestValidatePasswordPolicy(t *testing.T) {
	if err := ValidatePasswordPolicy("admin123"); err != nil {
		t.Fatalf("ValidatePasswordPolicy() error = %v", err)
	}
	if err := ValidatePasswordPolicy("short"); err == nil {
		t.Fatal("expected short password to fail")
	}
	if err := ValidatePasswordPolicy("        "); err == nil {
		t.Fatal("expected whitespace-only password to fail")
	}
}
