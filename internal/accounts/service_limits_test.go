package accounts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DevNewbie1826/omo-webchat/internal/omorpc"
)

type longLabelEngine struct{ *fakeEngine }

func (e *longLabelEngine) Call(ctx context.Context, cmd omorpc.Command) (*omorpc.Response, error) {
	var data any
	switch cmd.(type) {
	case omorpc.GetAuthProviders:
		data = map[string]any{"providers": []map[string]any{{"id": "anthropic", "name": strings.Repeat("한", 100), "keyHint": "sk-ant-SECRET-abcdef\n\t\u200b가나다라 "}}}
	case omorpc.GetProviderAccounts:
		data = map[string]any{"accounts": []map[string]string{{"name": strings.Repeat("계", 100), "displayName": strings.Repeat("정", 100)}}}
	default:
		return e.fakeEngine.Call(ctx, cmd)
	}
	raw, err := json.Marshal(data)
	return &omorpc.Response{Success: true, Data: raw}, err
}
func TestListBoundsEngineLabelsAndKeyHint(t *testing.T) {
	e := &longLabelEngine{newFake()}
	s := New(context.Background(), t.TempDir(), func(context.Context) (Engine, error) { return e, nil })
	providers, err := s.List(context.Background())
	if err != nil || len(providers) != 1 {
		t.Fatalf("List: %v, rows=%d", err, len(providers))
	}
	p := providers[0]
	if p.Name != strings.Repeat("한", 80) || p.Accounts[0].Name != strings.Repeat("계", 100) || p.Accounts[0].DisplayName != strings.Repeat("정", 80) || p.KeyHint != "가나다라" {
		t.Fatalf("unbounded projection: %+v", p)
	}
}
func TestTrailingVisibleHint(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{"", ""}, {"abc", "abc"}, {"secret-abcdef", "cdef"},
		{"abcdef\n\t\x00\u200b ", "cdef"}, {"가나다라마", "나다라마"},
	} {
		if got := trailingVisibleHint(tt.input); got != tt.want {
			t.Errorf("hint=%q want=%q", got, tt.want)
		}
	}
	if limitAccountLabel("short") != "short" {
		t.Fatal("short label changed")
	}
}
