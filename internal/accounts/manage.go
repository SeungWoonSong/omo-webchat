package accounts

import (
	"context"
	"strings"
	"time"

	"github.com/DevNewbie1826/omo-webchat/internal/omorpc"
)

// Change applies only engine-supported mutations. Names are immutable engine IDs;
// nil Name releases a pin and restores the engine's automatic account selection.
type Change struct {
	Action string
	Name   *string
}

func (s *Service) Change(ctx context.Context, provider string, change Change) error {
	if !supported(provider) || (change.Action != "remove" && change.Action != "pin") ||
		(change.Action == "remove" && change.Name == nil) ||
		(change.Name != nil && strings.TrimSpace(*change.Name) == "") {
		return ErrInvalid
	}
	// Serialize mutations with login setup for this provider.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[provider] != nil {
		return ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	e, sid, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer closeEngine(ctx, e, sid)
	switch change.Action {
	case "remove":
		return call(ctx, e, omorpc.AccountRemove{SessionID: sid, Provider: provider, Name: *change.Name}, nil)
	case "pin":
		return call(ctx, e, omorpc.AccountPin{SessionID: sid, Provider: provider, Name: change.Name}, nil)
	default:
		return ErrInvalid
	}
}
