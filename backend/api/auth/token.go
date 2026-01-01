package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	TokenLength        = 32
	TokenPrefix        = "ct_"
	SetupCodeLength    = 8
	SetupCodeAlphabet  = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	DefaultSetupExpiry = 20 * time.Minute
	MaxSetupExpiry     = 72 * time.Hour
	DefaultTokenExpiry = 90 * 24 * time.Hour
	MaxTokenExpiry     = 365 * 24 * time.Hour
)

type TokenProvider struct {
	mu           sync.RWMutex
	pendingCodes map[string]*PendingSetupCode
}

type PendingSetupCode struct {
	Code        string
	TokenName   string
	TokenExpiry time.Duration
	ExpiresAt   time.Time
}

func NewTokenProvider() *TokenProvider {
	return &TokenProvider{
		pendingCodes: make(map[string]*PendingSetupCode),
	}
}

func (p *TokenProvider) GenerateToken() (plaintext string, hash []byte, error error) {
	randomBytes := make([]byte, TokenLength)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(randomBytes)
	plaintext = TokenPrefix + encoded

	hash = p.HashToken(plaintext)

	return plaintext, hash, nil
}

func (p *TokenProvider) HashToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func (p *TokenProvider) ValidateTokenFormat(token string) bool {
	if !strings.HasPrefix(token, TokenPrefix) {
		return false
	}

	encoded := strings.TrimPrefix(token, TokenPrefix)
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}

	return len(decoded) == TokenLength
}

func (p *TokenProvider) CreateSetupCode(tokenName string, codeExpiry, tokenExpiry time.Duration) (*PendingSetupCode, error) {
	if codeExpiry > MaxSetupExpiry {
		return nil, fmt.Errorf("code expiry exceeds maximum of %v", MaxSetupExpiry)
	}
	if tokenExpiry > MaxTokenExpiry {
		return nil, fmt.Errorf("token expiry exceeds maximum of %v", MaxTokenExpiry)
	}

	codeBytes := make([]byte, SetupCodeLength)
	_, err := rand.Read(codeBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	alphabetLen := len(SetupCodeAlphabet)
	var code strings.Builder
	for i, b := range codeBytes {
		if i == 4 {
			code.WriteRune('-')
		}
		code.WriteByte(SetupCodeAlphabet[int(b)%alphabetLen])
	}

	setupCode := &PendingSetupCode{
		Code:        code.String(),
		TokenName:   tokenName,
		TokenExpiry: tokenExpiry,
		ExpiresAt:   time.Now().Add(codeExpiry),
	}

	p.mu.Lock()
	p.pendingCodes[strings.ToUpper(setupCode.Code)] = setupCode
	p.mu.Unlock()

	return setupCode, nil
}

func (p *TokenProvider) ConsumeSetupCode(code string) *PendingSetupCode {
	normalizedCode := strings.ToUpper(code)

	p.mu.Lock()
	defer p.mu.Unlock()

	setupCode, exists := p.pendingCodes[normalizedCode]
	if !exists {
		return nil
	}

	if time.Now().After(setupCode.ExpiresAt) {
		delete(p.pendingCodes, normalizedCode)
		return nil
	}

	delete(p.pendingCodes, normalizedCode)
	return setupCode
}
