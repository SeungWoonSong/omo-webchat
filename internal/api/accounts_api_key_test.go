package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DevNewbie1826/omo-webchat/internal/accounts"
	"github.com/DevNewbie1826/omo-webchat/internal/auth"
	"github.com/DevNewbie1826/omo-webchat/internal/config"
	"github.com/DevNewbie1826/omo-webchat/internal/omorpc"
	"github.com/DevNewbie1826/omo-webchat/internal/omorpc/omorpctest"
)

type apiKeyEngine struct {
	mode   string
	called bool
}

func (e *apiKeyEngine) Call(_ context.Context, cmd omorpc.Command) (*omorpc.Response, error) {
	data := json.RawMessage(`{}`)
	switch c := cmd.(type) {
	case omorpc.OpenSession:
		data = json.RawMessage(`{"sessionId":"private"}`)
	case omorpc.LoginAPIKey:
		e.called = true
		switch e.mode {
		case "transport":
			return nil, errors.New(c.Key)
		case "engine":
			return &omorpc.Response{Success: false, Error: c.Key}, nil
		case "malformed":
			data = json.RawMessage(`{"secret":"` + c.Key + `"`)
		case "replace":
			data = json.RawMessage(`{"replaced":"oauth","backupPath":"/private/` + c.Key + `"}`)
		default:
			data = nil
		}
	}
	return &omorpc.Response{Success: true, Data: data}, nil
}
func (*apiKeyEngine) Notify(context.Context, omorpc.Notification) error { return nil }
func (*apiKeyEngine) Events() <-chan *omorpc.Event                      { return nil }
func (*apiKeyEngine) Close() error                                      { return nil }

func TestAccountAPIKeyEndpointSecretsAndAuth(t *testing.T) {
	const sentinel = "sk-ant-test-SENTINEL-000000000000abcd"
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	oldLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(oldLogger)
	oldOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(oldOutput)
	for _, mode := range []string{"success", "replace", "transport", "engine", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sessions := auth.NewSessionStore(ctx, "test-only", logger)
			token, err := sessions.Create(ctx)
			if err != nil {
				t.Fatal(err)
			}
			e := &apiKeyEngine{mode: mode}
			server := New(ctx, &config.Config{Root: t.TempDir()}, nil, sessions, nil, http.NotFoundHandler(), logger)
			server.accounts = accounts.New(ctx, t.TempDir(), func(context.Context) (accounts.Engine, error) { return e, nil })
			handler := server.Handler()
			send := func(provider, body string, authenticated bool) *httptest.ResponseRecorder {
				r := httptest.NewRequest("POST", "/api/accounts/"+provider+"/api-key", strings.NewReader(body))
				if authenticated {
					r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if strings.Contains(w.Body.String(), sentinel) || strings.Contains(w.Header().Get("Location"), sentinel) || strings.Contains(logs.String(), sentinel) {
					t.Fatal("sentinel leaked")
				}
				return w
			}
			body := `{"key":"` + sentinel + `"}`
			if send("anthropic", body, false).Code != 401 || e.called {
				t.Fatal("endpoint not protected")
			}
			if send("cursor", body, true).Code != 400 || e.called {
				t.Fatal("provider allowlist not enforced")
			}
			if send("anthropic", `{"key":"wrong"}`, true).Code != 400 {
				t.Fatal("invalid key accepted")
			}
			if send("anthropic", `{"key":"`+strings.Repeat("a", 5000)+`"}`, true).Code != 400 {
				t.Fatal("oversized body accepted")
			}
			if send("anthropic", body+` {}`, true).Code != 400 {
				t.Fatal("trailing JSON accepted")
			}
			w := send("anthropic", body, true)
			want := 502
			if mode == "success" || mode == "replace" {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("status=%d want=%d", w.Code, want)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cache enabled")
			}
			if want == 200 {
				var result map[string]any
				if json.Unmarshal(w.Body.Bytes(), &result) != nil || result["ok"] != true || result["keyHint"] != "abcd" {
					t.Fatal("invalid result")
				}
				if _, ok := result["backupPath"]; ok {
					t.Fatal("backup path leaked")
				}
				if mode == "replace" && (result["replaced"] != "oauth" || result["backedUp"] != true) {
					t.Fatal("replacement lost")
				}
			}
		})
	}
}

// Exercise frame encoding, socket writes, response decoding and error wrapping,
// not just the service's Engine seam. All data belongs to the isolated daemon.
func TestAccountAPIKeyRPCSecrets(t *testing.T) {
	const sentinel = "sk-ant-test-WIRE-SENTINEL-000000000000abcd"
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	previous := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(previous)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	daemon := omorpctest.New(t.TempDir())
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	defer daemon.Stop()
	sessions := auth.NewSessionStore(ctx, "test-only", logger)
	token, err := sessions.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	server := New(ctx, &config.Config{Root: t.TempDir()}, nil, sessions, nil, http.NotFoundHandler(), logger)
	server.accounts = accounts.New(ctx, t.TempDir(), func(ctx context.Context) (accounts.Engine, error) {
		return omorpc.DialWithConfig(ctx, daemon.SocketPath(), omorpc.Config{NoReconnect: true})
	})
	for _, failure := range []bool{false, true} {
		if failure {
			daemon.FailNext("login_api_key", sentinel)
		}
		request := httptest.NewRequest("POST", "/api/accounts/anthropic/api-key", strings.NewReader(`{"key":"`+sentinel+`"}`))
		request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		want := 200
		if failure {
			want = 502
		}
		if response.Code != want {
			t.Fatalf("status=%d want=%d", response.Code, want)
		}
		if strings.Contains(response.Body.String(), sentinel) || strings.Contains(logs.String(), sentinel) {
			t.Fatal("RPC sentinel leaked")
		}
	}
}
