package auth

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateToken(t *testing.T) {
	provider := NewTokenProvider()

	plaintext, hash, err := provider.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() failed: %v", err)
	}

	if !strings.HasPrefix(plaintext, TokenPrefix) {
		t.Errorf("token does not have prefix %q: %q", TokenPrefix, plaintext)
	}

	if len(hash) != 32 {
		t.Errorf("hash length = %d, want 32", len(hash))
	}

	plaintext2, hash2, err := provider.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() second call failed: %v", err)
	}

	if plaintext == plaintext2 {
		t.Error("GenerateToken() produced duplicate tokens")
	}

	if string(hash) == string(hash2) {
		t.Error("GenerateToken() produced duplicate hashes")
	}
}

func TestHashToken(t *testing.T) {
	provider := NewTokenProvider()

	token := "ct_test1234567890"
	hash1 := provider.HashToken(token)
	hash2 := provider.HashToken(token)

	if len(hash1) != 32 {
		t.Errorf("hash length = %d, want 32", len(hash1))
	}

	if string(hash1) != string(hash2) {
		t.Error("HashToken() produced inconsistent hashes for same input")
	}

	differentToken := "ct_different"
	hash3 := provider.HashToken(differentToken)

	if string(hash1) == string(hash3) {
		t.Error("HashToken() produced same hash for different tokens")
	}
}

func TestValidateTokenFormat(t *testing.T) {
	provider := NewTokenProvider()

	tests := []struct {
		name  string
		token string
		valid bool
	}{
		{
			name:  "valid token from generator",
			token: mustGenerateToken(t, provider),
			valid: true,
		},
		{
			name:  "missing prefix",
			token: "abc123",
			valid: false,
		},
		{
			name:  "wrong prefix",
			token: "tk_abc123",
			valid: false,
		},
		{
			name:  "invalid base64",
			token: "ct_!!!invalid",
			valid: false,
		},
		{
			name:  "wrong length",
			token: "ct_abc",
			valid: false,
		},
		{
			name:  "empty",
			token: "",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := provider.ValidateTokenFormat(tt.token)
			if valid != tt.valid {
				t.Errorf("ValidateTokenFormat(%q) = %v, want %v", tt.token, valid, tt.valid)
			}
		})
	}
}

func TestCreateSetupCode(t *testing.T) {
	provider := NewTokenProvider()

	setupCode, err := provider.CreateSetupCode("test-token", 5*time.Minute, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("CreateSetupCode() failed: %v", err)
	}

	if setupCode.Code == "" {
		t.Error("setup code is empty")
	}

	if len(setupCode.Code) != 9 {
		t.Errorf("setup code length = %d, want 9 (XXXX-XXXX)", len(setupCode.Code))
	}

	if setupCode.Code[4] != '-' {
		t.Errorf("setup code format incorrect, missing hyphen at position 4: %q", setupCode.Code)
	}

	if setupCode.TokenName != "test-token" {
		t.Errorf("TokenName = %q, want %q", setupCode.TokenName, "test-token")
	}

	if setupCode.TokenExpiry != 90*24*time.Hour {
		t.Errorf("TokenExpiry = %v, want %v", setupCode.TokenExpiry, 90*24*time.Hour)
	}

	for i, ch := range setupCode.Code {
		if i == 4 {
			continue
		}
		if !strings.ContainsRune(SetupCodeAlphabet, ch) {
			t.Errorf("setup code contains invalid character %q at position %d", ch, i)
		}
	}
}

func TestCreateSetupCodeExpiry(t *testing.T) {
	provider := NewTokenProvider()

	_, err := provider.CreateSetupCode("test", MaxSetupExpiry+time.Hour, DefaultTokenExpiry)
	if err == nil {
		t.Error("CreateSetupCode() with code expiry > max should fail")
	}

	_, err = provider.CreateSetupCode("test", DefaultSetupExpiry, MaxTokenExpiry+time.Hour)
	if err == nil {
		t.Error("CreateSetupCode() with token expiry > max should fail")
	}
}

func TestConsumeSetupCode(t *testing.T) {
	provider := NewTokenProvider()

	setupCode, err := provider.CreateSetupCode("test-token", 5*time.Minute, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("CreateSetupCode() failed: %v", err)
	}

	consumed := provider.ConsumeSetupCode(setupCode.Code)
	if consumed == nil {
		t.Fatal("ConsumeSetupCode() returned nil")
	}

	if consumed.TokenName != "test-token" {
		t.Errorf("consumed TokenName = %q, want %q", consumed.TokenName, "test-token")
	}

	consumed2 := provider.ConsumeSetupCode(setupCode.Code)
	if consumed2 != nil {
		t.Error("ConsumeSetupCode() should return nil for already consumed code")
	}
}

func TestConsumeSetupCodeCaseInsensitive(t *testing.T) {
	provider := NewTokenProvider()

	setupCode, err := provider.CreateSetupCode("test-token", 5*time.Minute, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("CreateSetupCode() failed: %v", err)
	}

	lowercase := strings.ToLower(setupCode.Code)
	consumed := provider.ConsumeSetupCode(lowercase)
	if consumed == nil {
		t.Error("ConsumeSetupCode() should be case-insensitive")
	}
}

func TestConsumeSetupCodeExpired(t *testing.T) {
	provider := NewTokenProvider()

	setupCode, err := provider.CreateSetupCode("test-token", 1*time.Millisecond, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("CreateSetupCode() failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	consumed := provider.ConsumeSetupCode(setupCode.Code)
	if consumed != nil {
		t.Error("ConsumeSetupCode() should return nil for expired code")
	}
}

func TestConsumeSetupCodeNotFound(t *testing.T) {
	provider := NewTokenProvider()

	consumed := provider.ConsumeSetupCode("ABCD-1234")
	if consumed != nil {
		t.Error("ConsumeSetupCode() should return nil for non-existent code")
	}
}

func mustGenerateToken(t *testing.T, provider *TokenProvider) string {
	t.Helper()
	plaintext, _, err := provider.GenerateToken()
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	return plaintext
}
