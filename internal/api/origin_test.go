package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DevNewbie1826/omo-webchat/internal/accounts"
	"github.com/DevNewbie1826/omo-webchat/internal/auth"
	"github.com/DevNewbie1826/omo-webchat/internal/config"
)

func TestRequireSameOrigin(t *testing.T) {
	for _, tt := range []struct {
		name, method, host, origin, site string
		want                             int
	}{
		{"same", "POST", "localhost:3197", "http://LOCALHOST:3197", "same-origin", 204},
		{"port", "POST", "localhost:3197", "http://localhost:8080", "same-site", 403},
		{"host", "PUT", "localhost:3197", "http://evil.test:3197", "", 403},
		{"null", "PATCH", "localhost:3197", "null", "", 403},
		{"cross site missing origin", "DELETE", "localhost", "", "cross-site", 403},
		{"same site missing origin", "POST", "localhost", "", "same-site", 403},
		{"non browser", "POST", "localhost", "", "", 204},
		{"same origin fetch", "POST", "localhost", "", "same-origin", 204},
		{"navigation", "POST", "localhost", "", "none", 204},
		{"get unaffected", "GET", "localhost", "null", "cross-site", 204},
		{"head unaffected", "HEAD", "localhost", "null", "cross-site", 204},
		{"default http", "POST", "localhost:80", "http://localhost", "", 204},
		{"default https proxy", "POST", "localhost", "https://localhost:443", "", 204},
		{"https proxy", "POST", "localhost", "https://localhost", "", 204},
		{"ipv6", "POST", "[::1]:80", "http://[::1]", "", 204},
		{"path", "POST", "localhost", "http://localhost/", "", 403},
		{"userinfo", "POST", "localhost", "http://user@localhost", "", 403},
		{"query", "POST", "localhost", "http://localhost?", "", 403},
		{"bad port", "POST", "localhost", "http://localhost:bad", "", 403},
		{"forwarded ignored", "POST", "localhost:3197", "https://localhost", "", 403},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "http://"+tt.host+"/api/test", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.site != "" {
				r.Header.Set("Sec-Fetch-Site", tt.site)
			}
			r.Header.Set("X-Forwarded-Proto", "https")
			w := httptest.NewRecorder()
			requireSameOrigin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("status=%d want=%d", w.Code, tt.want)
			}
			if tt.want == 403 && strings.TrimSpace(w.Body.String()) != `{"error":"cross_origin"}` {
				t.Fatal(w.Body.String())
			}
		})
	}
	for _, origins := range [][]string{{""}, {"http://localhost", "http://localhost"}} {
		r := httptest.NewRequest("POST", "http://localhost/api/test", nil)
		r.Header["Origin"] = origins
		if sameOriginRequest(r) {
			t.Fatal("invalid Origin headers accepted")
		}
	}
}

func TestAccountAPIKeyCrossPortSimplePOST(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sessions := auth.NewSessionStore(ctx, "test-only", logger)
	token, err := sessions.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	e := &apiKeyEngine{}
	server := New(ctx, &config.Config{Root: t.TempDir()}, nil, sessions, nil, http.NotFoundHandler(), logger)
	server.accounts = accounts.New(ctx, t.TempDir(), func(context.Context) (accounts.Engine, error) { return e, nil })
	for _, tt := range []struct {
		origin, contentType string
		want                int
	}{
		{"http://127.0.0.1:8080", "text/plain", 403},
		{"http://127.0.0.1:8080", "application/json", 403},
		{"http://127.0.0.1:3197", "text/plain", 415},
		{"http://127.0.0.1:3197", "multipart/form-data; boundary=test", 415},
	} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:3197/api/accounts/anthropic/api-key", strings.NewReader(`{"key":"sk-ant-test-000000000000abcd"}`))
		r.Header.Set("Origin", tt.origin)
		r.Header.Set("Content-Type", tt.contentType)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		if w.Code != tt.want || e.called {
			t.Fatalf("status=%d want=%d engine_called=%v", w.Code, tt.want, e.called)
		}
	}
}
