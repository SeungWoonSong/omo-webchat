package omorpc

const (
	CmdGetAuthProviders    = "get_auth_providers"
	CmdGetProviderAccounts = "get_provider_accounts"
	CmdLoginStart          = "login_start"
	CmdLoginCancel         = "login_cancel"
	EventAuthLoginURL      = "auth_login_url"
	EventAuthLoginEnd      = "auth_login_end"
)

type GetAuthProviders struct {
	SessionID string `json:"sessionId"`
}

func (GetAuthProviders) commandName() string { return CmdGetAuthProviders }

type GetProviderAccounts struct {
	SessionID string `json:"sessionId"`
	Provider  string `json:"provider"`
}

func (GetProviderAccounts) commandName() string { return CmdGetProviderAccounts }

type LoginStart struct {
	SessionID string `json:"sessionId"`
	Provider  string `json:"provider"`
}

func (LoginStart) commandName() string { return CmdLoginStart }

type LoginCancel struct {
	SessionID string `json:"sessionId"`
	Provider  string `json:"provider"`
}

func (LoginCancel) commandName() string { return CmdLoginCancel }

type AccountRemove struct {
	SessionID string `json:"sessionId"`
	Provider  string `json:"provider"`
	Name      string `json:"name"`
}

func (AccountRemove) commandName() string { return "account_remove" }

type AccountPin struct {
	SessionID string  `json:"sessionId"`
	Provider  string  `json:"provider"`
	Name      *string `json:"name"`
}

func (AccountPin) commandName() string { return "account_pin" }

// AuthLoginEvent contains only browser-facing login information, never credentials.
type AuthLoginEvent struct {
	Type        string   `json:"type"`
	Provider    string   `json:"provider,omitempty"`
	URL         string   `json:"url,omitempty"`
	ID          string   `json:"id,omitempty"`
	Method      string   `json:"method,omitempty"`
	Title       string   `json:"title,omitempty"`
	Message     string   `json:"message,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
	Success     bool     `json:"success,omitempty"`
	Error       string   `json:"error,omitempty"`
}
