package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DevNewbie1826/omo-webchat/internal/omorpc"
)

func TestValidateAPIKey(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		valid       bool
	}{
		{"empty", "", false}, {"whitespace", " \t", false}, {"internal space", "sk-ant-a b", false},
		{"newline", "sk-ant-a\nb", false}, {"control", "sk-ant-a\x00b", false}, {"unicode space", "sk-ant-a\u2003b", false},
		{"too long", "sk-ant-" + strings.Repeat("a", 506), false}, {"bytes", "sk-ant-" + strings.Repeat("한", 169), false},
		{"wrong prefix", "sk-other-abcd", false}, {"valid", "sk-ant-test-000000000000abcd", true},
		{"trim", " \tsk-ant-test-000000000000abcd\r\n", true}, {"max", "sk-ant-" + strings.Repeat("a", 505), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key, err := ValidateAPIKey(tt.input)
			if (err == nil) != tt.valid {
				t.Fatal("validation mismatch")
			}
			if tt.valid && key != strings.TrimSpace(tt.input) {
				t.Fatal("not trimmed")
			}
		})
	}
}

type keyEngine struct {
	*fakeEngine
	replacement bool
	fail        bool
	saved       bool
}

func (e *keyEngine) Call(ctx context.Context, cmd omorpc.Command) (*omorpc.Response, error) {
	switch c := cmd.(type) {
	case omorpc.LoginAPIKey:
		if c.Provider != "anthropic" || c.SessionID != "private" || c.Key != "sk-ant-test-000000000000abcd" {
			return nil, errors.New("bad command")
		}
		e.saved = true
		if e.fail {
			return &omorpc.Response{Success: false, Error: c.Key}, errors.New(c.Key)
		}
		if e.replacement {
			return &omorpc.Response{Success: true, Data: json.RawMessage(`{"replaced":"oauth","backupPath":"/private/backup"}`)}, nil
		}
		return &omorpc.Response{Success: true}, nil
	case omorpc.GetAuthProviders:
		return &omorpc.Response{Success: true, Data: json.RawMessage(`{"providers":[{"id":"anthropic-subscription"},{"id":"cursor"},{"id":"anthropic","authType":"api_key","status":{"configured":false}},{"id":"anthropic","authType":"oauth","keyHint":"abcd","status":{"configured":true}}]}`)}, nil
	}
	return e.fakeEngine.Call(ctx, cmd)
}
func TestLoginAPIKeyAndProviderManagement(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		e := &keyEngine{fakeEngine: newFake(), replacement: replacement}
		s := New(context.Background(), t.TempDir(), func(context.Context) (Engine, error) { return e, nil })
		result, err := s.LoginAPIKey(context.Background(), "anthropic", "  sk-ant-test-000000000000abcd  ")
		if err != nil || !result.OK || result.KeyHint != "abcd" || (result.Replaced == "oauth") != replacement || result.BackedUp != replacement {
			t.Fatal("incorrect key result")
		}
		rows, err := s.List(context.Background())
		if err != nil || len(rows) != 3 || rows[1].ID != "anthropic" || rows[1].KeyHint != "abcd" || rows[1].AuthType != "oauth" {
			t.Fatal("missing distinct API provider or configured auth method")
		}
		name := "own"
		for _, action := range []string{"pin", "remove"} {
			if err := s.Change(context.Background(), "anthropic", Change{Action: action, Name: &name}); err != nil {
				t.Fatal(err)
			}
		}
		if len(e.changes) != 2 {
			t.Fatal("mutations not sent")
		}
		select {
		case <-e.closed:
		default:
			t.Fatal("private connection not closed")
		}
	}
}
func TestLoginAPIKeyAllowlistAndErrors(t *testing.T) {
	e := &keyEngine{fakeEngine: newFake(), fail: true}
	s := New(context.Background(), t.TempDir(), func(context.Context) (Engine, error) { return e, nil })
	for _, p := range []string{"cursor", "anthropic-subscription", "openai", ""} {
		if _, err := s.LoginAPIKey(context.Background(), p, "sk-ant-test-000000000000abcd"); !errors.Is(err, ErrInvalid) {
			t.Fatal("API allowlist widened")
		}
	}
	if e.saved {
		t.Fatal("invalid provider reached engine")
	}
	if _, err := s.LoginAPIKey(context.Background(), "anthropic", "sk-ant-test-000000000000abcd"); err != ErrUnavailable {
		t.Fatal("private engine text escaped")
	}
	if _, err := s.Start(context.Background(), "owner", "anthropic"); err != ErrInvalid {
		t.Fatal("API key provider admitted to subscription login")
	}
}
