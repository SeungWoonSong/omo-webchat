package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DevNewbie1826/omo-webchat/internal/accounts"
	"github.com/DevNewbie1826/omo-webchat/internal/auth"
)

func (s *Server) accountError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, accounts.ErrBusy):
		status = http.StatusConflict
	case errors.Is(err, accounts.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, accounts.ErrInvalid), errors.Is(err, accounts.ErrAPIKey):
		status = http.StatusBadRequest
	}
	writeError(w, status, err.Error())
}

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.accounts == nil {
		s.accountError(w, accounts.ErrUnavailable)
		return
	}
	providers, err := s.accounts.List(r.Context())
	if err != nil {
		s.accountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

func (s *Server) handleAccountStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.accounts == nil {
		s.accountError(w, accounts.ErrUnavailable)
		return
	}
	cookie, _ := r.Cookie(auth.CookieName) // Protected router already validated it.
	f, err := s.accounts.Start(r.Context(), cookie.Value, r.PathValue("provider"))
	if err != nil {
		s.accountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": f.ID})
}

func (s *Server) handleAccountChange(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.accounts == nil {
		s.accountError(w, accounts.ErrUnavailable)
		return
	}
	var req struct {
		Name *string `json:"name"`
	}
	if decodeJSON(r, &req) != nil {
		s.accountError(w, accounts.ErrInvalid)
		return
	}
	err := s.accounts.Change(r.Context(), r.PathValue("provider"), accounts.Change{
		Action: r.PathValue("action"), Name: req.Name,
	})
	if err != nil {
		s.accountError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) accountFlow(w http.ResponseWriter, r *http.Request) *accounts.Flow {
	w.Header().Set("Cache-Control", "no-store")
	if s.accounts == nil {
		s.accountError(w, accounts.ErrUnavailable)
		return nil
	}
	cookie, _ := r.Cookie(auth.CookieName)
	f, err := s.accounts.Find(cookie.Value, r.PathValue("loginId"))
	if err != nil {
		s.accountError(w, err)
		return nil
	}
	return f
}

func (s *Server) handleAccountEvents(w http.ResponseWriter, r *http.Request) {
	f := s.accountFlow(w, r)
	if f == nil {
		return
	}
	defer f.Close() // Closing the panel/stream aborts an unfinished login.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		var data []byte
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			data = []byte(": keepalive\n\n")
		case ev, ok := <-f.Events():
			if !ok {
				return
			}
			b, err := json.Marshal(ev)
			if err != nil {
				return
			}
			data = []byte(fmt.Sprintf("data: %s\n\n", b))
		}
		if _, err := w.Write(data); err != nil {
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}

func (s *Server) handleAccountResponse(w http.ResponseWriter, r *http.Request) {
	f := s.accountFlow(w, r)
	if f == nil {
		return
	}
	var req struct {
		ID        string  `json:"id"`
		Value     *string `json:"value"`
		Confirmed *bool   `json:"confirmed"`
	}
	if decodeJSON(r, &req) != nil {
		s.accountError(w, accounts.ErrInvalid)
		return
	}
	if err := f.Respond(r.Context(), req.ID, req.Value, req.Confirmed); err != nil {
		s.accountError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAccountCancel(w http.ResponseWriter, r *http.Request) {
	f := s.accountFlow(w, r)
	if f == nil {
		return
	}
	if err := f.Cancel(r.Context()); err != nil {
		s.accountError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAccountAPIKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.PathValue("provider") != "anthropic" {
		s.accountError(w, accounts.ErrInvalid)
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	defer func() { req.Key = "" }()
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	if decoder.Decode(&req) != nil {
		s.accountError(w, accounts.ErrAPIKey)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		s.accountError(w, accounts.ErrAPIKey)
		return
	}
	if _, err := accounts.ValidateAPIKey(req.Key); err != nil {
		s.accountError(w, err)
		return
	}
	if s.accounts == nil {
		s.accountError(w, accounts.ErrUnavailable)
		return
	}
	result, err := s.accounts.LoginAPIKey(r.Context(), "anthropic", req.Key)
	req.Key = ""
	if err != nil {
		s.accountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
