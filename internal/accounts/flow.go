package accounts

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/DevNewbie1826/omo-webchat/internal/omorpc"
)

type Flow struct {
	ID                   string
	owner, provider, sid string
	engine               Engine
	events               chan omorpc.AuthLoginEvent
	cancel               context.CancelFunc
	mu                   sync.Mutex
	pending              map[string]string
}

func (f *Flow) Events() <-chan omorpc.AuthLoginEvent { return f.events }
func (f *Flow) Close()                               { f.cancel() }

func (f *Flow) Cancel(ctx context.Context) error {
	return call(ctx, f.engine, omorpc.LoginCancel{SessionID: f.sid, Provider: f.provider}, nil)
}

func (f *Flow) Respond(ctx context.Context, id string, value *string, confirmed *bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	method, ok := f.pending[id]
	if !ok || (method == "confirm" && confirmed == nil) || (method != "confirm" && value == nil) {
		return ErrInvalid
	}
	var raw json.RawMessage
	if value != nil {
		raw, _ = json.Marshal(*value)
	}
	if err := f.engine.Notify(ctx, omorpc.ExtensionUIResponse{SessionID: f.sid, ID: id, Value: raw, Confirmed: confirmed}); err != nil {
		return ErrUnavailable
	}
	delete(f.pending, id)
	return nil
}

// Only the initiating browser's authenticated cookie can access a flow.
func (s *Service) Find(owner, id string) (*Flow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f := s.flows[id]; f != nil && f.owner == owner {
		return f, nil
	}
	return nil, ErrNotFound
}
