package api

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// requireSameOrigin follows the WebSocket Host rule: forwarded headers are
// untrusted and TLS termination may make the upstream scheme differ. Explicit
// ports remain significant, except for the Origin scheme's default port.
func requireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !sameOriginRequest(r) {
				writeError(w, http.StatusForbidden, "cross_origin")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func sameOriginRequest(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		switch r.Header.Get("Sec-Fetch-Site") {
		case "", "same-origin", "none":
			return true
		default:
			return false
		}
	}
	if len(origins) != 1 || origins[0] == "" {
		return false
	}
	o, err := url.Parse(origins[0])
	if err != nil || o.Host == "" || o.User != nil || o.Path != "" || o.RawQuery != "" || o.ForceQuery || o.Fragment != "" || o.RawFragment != "" {
		return false
	}
	scheme := strings.ToLower(o.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}
	request, err := url.Parse(scheme + "://" + r.Host)
	if err != nil || request.Host != r.Host || request.Hostname() == "" {
		return false
	}
	return strings.EqualFold(originAuthority(o, scheme), originAuthority(request, scheme))
}

func originAuthority(u *url.URL, scheme string) string {
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}
