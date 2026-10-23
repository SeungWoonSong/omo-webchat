package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DevNewbie1826/omo-webchat/internal/omorpc"
)

// fakeEngine preserves the event-before-reply ordering of the real RPC engine.
type fakeEngine struct {
	events         chan *omorpc.Event
	answers        chan omorpc.ExtensionUIResponse
	closed         chan struct{}
	once           sync.Once
	changes        []omorpc.Command
	failChange     bool
	accountRemoved bool
}

func newFake() *fakeEngine {
	return &fakeEngine{events: make(chan *omorpc.Event, 16), answers: make(chan omorpc.ExtensionUIResponse, 1), closed: make(chan struct{})}
}
func (e *fakeEngine) emit(event omorpc.AuthLoginEvent) {
	raw, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	e.events <- &omorpc.Event{Type: event.Type, SessionID: "private", Raw: raw}
}
func (e *fakeEngine) Call(_ context.Context, cmd omorpc.Command) (*omorpc.Response, error) {
	data := json.RawMessage(`{}`)
	switch c := cmd.(type) {
	case omorpc.OpenSession:
		data = json.RawMessage(`{"sessionId":"private"}`)
	case omorpc.GetAuthProviders:
		data = json.RawMessage(`{"providers":[{"id":"chatgpt-subscription","name":"ChatGPT","status":{"configured":true},"token":"must-not-leak"},{"id":"openai","name":"API key"},{"id":"chatgpt-subscription","name":"Duplicate registration"}]}`)
	case omorpc.GetProviderAccounts:
		data = json.RawMessage(`{"accounts":[{"name":"own","source":"oauth","accessToken":"must-not-leak"}]}`)
		if e.accountRemoved {
			data = json.RawMessage(`{"accounts":[]}`)
		}
	case omorpc.LoginStart:
		e.emit(omorpc.AuthLoginEvent{Type: omorpc.EventAuthLoginURL, Provider: c.Provider, URL: "https://example.test/login"})
		e.emit(omorpc.AuthLoginEvent{Type: "extension_ui_request", ID: "code", Method: "input", Title: "Code"})
	case omorpc.LoginCancel:
		e.emit(omorpc.AuthLoginEvent{Type: omorpc.EventAuthLoginEnd, Provider: c.Provider})
	case omorpc.AccountRemove, omorpc.AccountPin:
		e.changes = append(e.changes, cmd)
		if e.failChange {
			return &omorpc.Response{Success: false, Error: "private engine text"}, nil
		}
		if _, ok := cmd.(omorpc.AccountRemove); ok {
			e.accountRemoved = true
		}
	}
	return &omorpc.Response{Success: true, Data: data}, nil
}
func (e *fakeEngine) Notify(_ context.Context, n omorpc.Notification) error {
	e.answers <- n.(omorpc.ExtensionUIResponse)
	e.emit(omorpc.AuthLoginEvent{Type: omorpc.EventAuthLoginEnd, Success: true})
	return nil
}
func (e *fakeEngine) Events() <-chan *omorpc.Event { return e.events }
func (e *fakeEngine) Close() error                 { e.once.Do(func() { close(e.closed) }); return nil }

func service(t *testing.T) (*Service, *fakeEngine, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	e := newFake()
	return New(ctx, t.TempDir(), func(context.Context) (Engine, error) { return e, nil }), e, ctx
}
func next(t *testing.T, ctx context.Context, f *Flow, kind string) omorpc.AuthLoginEvent {
	t.Helper()
	select {
	case event, ok := <-f.Events():
		if !ok || event.Type != kind {
			t.Fatalf("event=%+v open=%v; want %s", event, ok, kind)
		}
		return event
	case <-ctx.Done():
		t.Fatal("event deadline")
		return omorpc.AuthLoginEvent{}
	}
}

func TestLoginRoundTrip(t *testing.T) {
	s, e, ctx := service(t)
	f, err := s.Start(ctx, "owner", "chatgpt-subscription")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if next(t, ctx, f, omorpc.EventAuthLoginURL).URL != "https://example.test/login" {
		t.Fatal("URL lost")
	}
	prompt := next(t, ctx, f, "extension_ui_request")
	code := "private-code"
	if err := f.Respond(ctx, prompt.ID, &code, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case answer := <-e.answers:
		if string(answer.Value) != `"private-code"` || answer.ID != "code" || answer.SessionID != "private" {
			t.Fatalf("incorrect answer routing: %+v", answer)
		}
	case <-ctx.Done():
		t.Fatal("answer deadline")
	}
	if !next(t, ctx, f, omorpc.EventAuthLoginEnd).Success {
		t.Fatal("login did not succeed")
	}
}

func TestCancelAndPrivateOwnership(t *testing.T) {
	s, _, ctx := service(t)
	f, err := s.Start(ctx, "owner", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := s.Find("other-client", f.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("other client can read login")
	}
	if _, err := s.Find("owner", f.ID); err != nil {
		t.Fatal(err)
	}
	next(t, ctx, f, omorpc.EventAuthLoginURL)
	next(t, ctx, f, "extension_ui_request")
	if err := f.Cancel(ctx); err != nil {
		t.Fatal(err)
	}
	if next(t, ctx, f, omorpc.EventAuthLoginEnd).Success {
		t.Fatal("cancel succeeded as login")
	}
}

func TestConcurrentStartBlocksExplicitly(t *testing.T) {
	s, _, ctx := service(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := s.Start(ctx, "owner", "anthropic-subscription")
			results <- err
		})
	}
	wg.Wait()
	a, b := <-results, <-results
	if !((a == nil && errors.Is(b, ErrBusy)) || (b == nil && errors.Is(a, ErrBusy))) {
		t.Fatalf("start results %v %v", a, b)
	}
}

func TestListProjectsOnlySecretFreeSubscriptionDescriptors(t *testing.T) {
	s, _, ctx := service(t)
	rows, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].Accounts) != 1 || rows[0].Accounts[0].Name != "own" {
		t.Fatalf("rows=%s", b)
	}
	var decoded []map[string]json.RawMessage
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded[0]["token"]; ok {
		t.Fatal("credential escaped projection")
	}
}

func TestDisconnectClosesEngine(t *testing.T) {
	s, e, ctx := service(t)
	f, err := s.Start(ctx, "owner", "cursor")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	select {
	case <-e.closed:
	case <-ctx.Done():
		t.Fatal("engine not closed on disconnect")
	}
}

func TestAccountChangeRoutesImmutableName(t *testing.T) {
	for _, action := range []string{"remove", "pin"} {
		t.Run(action, func(t *testing.T) {
			s, e, ctx := service(t)
			name := "saved-account"
			if err := s.Change(ctx, "chatgpt-subscription", Change{Action: action, Name: &name}); err != nil {
				t.Fatal(err)
			}
			if len(e.changes) != 1 {
				t.Fatal("mutation not sent")
			}
			b, err := json.Marshal(e.changes[0])
			if err != nil {
				t.Fatal(err)
			}
			var got struct{ SessionID, Provider, Name string }
			if json.Unmarshal(b, &got) != nil || got.SessionID != "private" || got.Provider != "chatgpt-subscription" || got.Name != name {
				t.Fatalf("incorrect mutation: %s", b)
			}
			select {
			case <-e.closed:
			default:
				t.Fatal("private connection leaked")
			}
		})
	}
}

func TestRemoveFailureIsNotReportedAsSuccess(t *testing.T) {
	s, e, ctx := service(t)
	e.failChange = true
	name := "saved-account"
	if err := s.Change(ctx, "cursor", Change{Action: "remove", Name: &name}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	if e.accountRemoved {
		t.Fatal("failed removal changed stored account")
	}
}

func TestRemovedAccountIsAbsentFromList(t *testing.T) {
	s, _, ctx := service(t)
	name := "own"
	if err := s.Change(ctx, "chatgpt-subscription", Change{Action: "remove", Name: &name}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.List(ctx)
	if err != nil || len(rows) != 1 || len(rows[0].Accounts) != 0 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestInvalidAccountChangeNeverReachesEngine(t *testing.T) {
	s, e, ctx := service(t)
	if err := s.Change(ctx, "cursor", Change{Action: "remove"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
	if len(e.changes) != 0 {
		t.Fatal("invalid removal reached engine")
	}
}

func TestUnpinSendsNullName(t *testing.T) {
	s, e, ctx := service(t)
	if err := s.Change(ctx, "cursor", Change{Action: "pin"}); err != nil {
		t.Fatal(err)
	}
	c, ok := e.changes[0].(omorpc.AccountPin)
	if !ok || c.Name != nil {
		t.Fatal("unpin did not send null")
	}
}
