// Package accounts owns private, short-lived RPC sessions for subscription login.
// Login input is sent only to the engine, never to chat, disk, or a logger.
package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"unicode"

	"github.com/DevNewbie1826/omo-webchat/internal/omorpc"
)

var (
	ErrBusy        = errors.New("a login for this provider is already active; cancel it first")
	ErrNotFound    = errors.New("login not found")
	ErrUnavailable = errors.New("account engine unavailable")
	ErrInvalid     = errors.New("invalid login request")
)

type Engine interface {
	Call(context.Context, omorpc.Command) (*omorpc.Response, error)
	Notify(context.Context, omorpc.Notification) error
	Events() <-chan *omorpc.Event
	Close() error
}

type Dial func(context.Context) (Engine, error)

type Service struct {
	ctx    context.Context
	dial   Dial
	cwd    string
	mu     sync.Mutex
	active map[string]*Flow
	flows  map[string]*Flow
}

type Account struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Source      string `json:"source"`
	Blocked     bool   `json:"blocked"`
	Pinned      bool   `json:"pinned"`
}

type Provider struct {
	ID       string `json:"id"`
	AuthType string `json:"authType,omitempty"`
	KeyHint  string `json:"keyHint,omitempty"`
	Name     string `json:"name"`
	Status   struct {
		Configured bool `json:"configured"`
	} `json:"status"`
	Accounts []Account `json:"accounts"`
}

func New(ctx context.Context, cwd string, dial Dial) *Service {
	return &Service{ctx: ctx, cwd: cwd, dial: dial, active: make(map[string]*Flow), flows: make(map[string]*Flow)}
}

func supported(p string) bool {
	return p == "anthropic" || subscriptionSupported(p)
}

func subscriptionSupported(p string) bool {
	return p == "anthropic-subscription" || p == "chatgpt-subscription" || p == "cursor"
}

func call(ctx context.Context, e Engine, cmd omorpc.Command, dst any) error {
	r, err := e.Call(ctx, cmd)
	// Never propagate arbitrary engine text: it may contain user input.
	if err != nil || r == nil || !r.Success {
		return ErrUnavailable
	}
	if dst != nil && json.Unmarshal(r.Data, dst) != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Service) open(ctx context.Context) (Engine, string, error) {
	e, err := s.dial(ctx)
	if err != nil {
		return nil, "", ErrUnavailable
	}
	var data omorpc.OpenSessionData
	if err := call(ctx, e, omorpc.OpenSession{CWD: s.cwd}, &data); err != nil {
		e.Close()
		return nil, "", err
	}
	return e, data.SessionID, nil
}

func closeEngine(ctx context.Context, e Engine, sid string) {
	c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	e.Call(c, omorpc.CloseSession{SessionID: sid})
	e.Close()
}

func (s *Service) List(ctx context.Context) ([]Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	e, sid, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	defer closeEngine(ctx, e, sid)
	var data struct {
		Providers []Provider `json:"providers"`
	}
	if err := call(ctx, e, omorpc.GetAuthProviders{SessionID: sid}, &data); err != nil {
		return nil, err
	}
	result := make([]Provider, 0, 4)
	seen := make(map[string]bool)
	// Auth methods can share an ID; retain the configured method for replacement
	// confirmation instead of blindly keeping the first OAuth/API-key row.
	for i, p := range data.Providers {
		if p.ID != "anthropic" || !p.Status.Configured {
			continue
		}
		for j := 0; j < i; j++ {
			if data.Providers[j].ID == p.ID {
				data.Providers[j], data.Providers[i] = p, data.Providers[j]
				break
			}
		}
	}
	for _, p := range data.Providers {
		if !supported(p.ID) || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		var accounts struct {
			Accounts []Account `json:"accounts"`
		}
		if err := call(ctx, e, omorpc.GetProviderAccounts{SessionID: sid, Provider: p.ID}, &accounts); err != nil {
			return nil, err
		}
		p.Name = limitAccountLabel(p.Name)
		p.KeyHint = trailingVisibleHint(p.KeyHint)
		p.Accounts = accounts.Accounts
		for i := range p.Accounts {
			p.Accounts[i].Name = limitAccountLabel(p.Accounts[i].Name)
			p.Accounts[i].DisplayName = limitAccountLabel(p.Accounts[i].DisplayName)
		}
		if p.Accounts == nil {
			p.Accounts = []Account{}
		}
		result = append(result, p)
	}
	// Keep the API-key lane adjacent to, but distinct from, Claude subscription.
	for i, p := range result {
		if p.ID != "anthropic" {
			continue
		}
		row := p
		result = append(result[:i], result[i+1:]...)
		at := 0
		for j, p := range result {
			if p.ID == "anthropic-subscription" {
				at = j + 1
				break
			}
		}
		result = append(result, Provider{})
		copy(result[at+1:], result[at:])
		result[at] = row
		break
	}
	return result, nil
}

func (s *Service) Start(ctx context.Context, owner, provider string) (*Flow, error) {
	if !subscriptionSupported(provider) {
		return nil, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[provider] != nil {
		return nil, ErrBusy
	}
	setup, stopSetup := context.WithTimeout(ctx, 30*time.Second)
	defer stopSetup()
	e, sid, err := s.open(setup)
	if err != nil {
		return nil, err
	}
	life, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
	f := &Flow{ID: omorpc.NewRequestID(), owner: owner, provider: provider, engine: e, sid: sid, events: make(chan omorpc.AuthLoginEvent, 32), cancel: cancel, pending: make(map[string]string)}
	s.active[provider] = f
	s.flows[f.ID] = f
	// Subscribe before the command, including events emitted before its reply.
	go s.consume(life, f)
	if err := call(setup, e, omorpc.LoginStart{SessionID: sid, Provider: provider}, nil); err != nil {
		cancel()
		return nil, err
	}
	return f, nil
}

func (s *Service) consume(ctx context.Context, f *Flow) {
	defer func() {
		closeEngine(ctx, f.engine, f.sid)
		s.mu.Lock()
		delete(s.active, f.provider)
		s.mu.Unlock()
		close(f.events)
		// Keep a fast terminal event available until the browser subscribes.
		// Disconnection, consumption, shutdown or the lease bounds retention.
		<-ctx.Done()
		s.mu.Lock()
		delete(s.flows, f.ID)
		s.mu.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			call(c, f.engine, omorpc.LoginCancel{SessionID: f.sid, Provider: f.provider}, nil)
			cancel()
			return
		case ev, ok := <-f.engine.Events():
			if !ok {
				return
			}
			if ev.SessionID != "" && ev.SessionID != f.sid {
				continue
			}
			switch ev.Type {
			case omorpc.EventAuthLoginURL, omorpc.EventAuthLoginEnd, "extension_ui_request":
			default:
				continue
			}
			var event omorpc.AuthLoginEvent
			if json.Unmarshal(ev.Raw, &event) != nil {
				continue
			}
			if event.Provider != "" && event.Provider != f.provider {
				continue
			}
			if event.Type == "extension_ui_request" {
				switch event.Method {
				case "input", "select", "confirm", "notify":
				default:
					continue
				}
				f.mu.Lock()
				f.pending[event.ID] = event.Method
				f.mu.Unlock()
			}
			select {
			case f.events <- event:
			case <-ctx.Done():
				return
			}
			if event.Type == omorpc.EventAuthLoginEnd {
				return
			}
		}
	}
}

func limitAccountLabel(value string) string {
	chars := []rune(value)
	if len(chars) > 80 {
		chars = chars[:80]
	}
	return string(chars)
}

func trailingVisibleHint(value string) string {
	chars := make([]rune, 0, 4)
	for _, char := range value {
		if unicode.IsPrint(char) && !unicode.IsSpace(char) {
			chars = append(chars, char)
		}
	}
	if len(chars) > 4 {
		chars = chars[len(chars)-4:]
	}
	return string(chars)
}
