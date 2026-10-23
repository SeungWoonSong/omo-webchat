package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/DevNewbie1826/omo-webchat/internal/omorpc"
)

var ErrAPIKey = errors.New("API key must start with sk-ant-, contain no whitespace or control characters, and be at most 512 bytes")

// ValidateAPIKey trims only surrounding whitespace; errors never include input.
func ValidateAPIKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 512 || !strings.HasPrefix(key, "sk-ant-") || strings.ContainsFunc(key, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", ErrAPIKey
	}
	return key, nil
}

type APIKeyResult struct {
	OK       bool   `json:"ok"`
	Replaced string `json:"replaced,omitempty"`
	KeyHint  string `json:"keyHint,omitempty"`
	BackedUp bool   `json:"backedUp,omitempty"`
}

func apiKeySupported(provider string) bool { return provider == "anthropic" }

func (s *Service) LoginAPIKey(ctx context.Context, provider, key string) (APIKeyResult, error) {
	defer func() { key = "" }()
	if !apiKeySupported(provider) {
		return APIKeyResult{}, ErrInvalid
	}
	var err error
	key, err = ValidateAPIKey(key)
	if err != nil {
		return APIKeyResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[provider] != nil {
		return APIKeyResult{}, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	e, sid, err := s.open(ctx)
	if err != nil {
		return APIKeyResult{}, err
	}
	defer closeEngine(ctx, e, sid)
	cmd := omorpc.LoginAPIKey{SessionID: sid, Provider: provider, Key: key}
	// Unlike call, success without data is valid here. Engine errors and arbitrary
	// data are never returned to the browser (including echoed keys/backup paths).
	response, err := e.Call(ctx, cmd)
	cmd.Key = ""
	if err != nil || response == nil || !response.Success {
		return APIKeyResult{}, ErrUnavailable
	}
	result := APIKeyResult{OK: true}
	if chars := []rune(key); len(chars) >= 12 {
		result.KeyHint = string(chars[len(chars)-4:])
	}
	var data struct {
		Replaced   string `json:"replaced"`
		BackupPath string `json:"backupPath"`
	}
	if len(response.Data) != 0 && string(response.Data) != "null" {
		if json.Unmarshal(response.Data, &data) != nil {
			return APIKeyResult{}, ErrUnavailable
		}
	}
	if data.Replaced == "oauth" {
		result.Replaced = "oauth"
		result.BackedUp = data.BackupPath != ""
	}
	return result, nil
}
