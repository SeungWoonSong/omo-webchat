import { apiJson, apiVoid } from "./api";

export interface AccountProvider {
  readonly id: string;
  readonly name: string;
  readonly status: { readonly configured: boolean };
  readonly accounts: readonly {
    readonly name: string;
    readonly displayName?: string;
    readonly source?: string;
    readonly blocked: boolean;
    readonly pinned?: boolean;
  }[];
}

export interface LoginEvent {
  readonly type: string;
  readonly url?: string | undefined;
  readonly id?: string | undefined;
  readonly method?: string | undefined;
  readonly title?: string | undefined;
  readonly message?: string | undefined;
  readonly placeholder?: string | undefined;
  readonly options?: readonly string[] | undefined;
  readonly success?: boolean;
  readonly error?: string | undefined;
}

export const listAccountProviders = (signal?: AbortSignal) =>
  apiJson<{ readonly providers: readonly AccountProvider[] }>("/api/accounts", signal ? { signal } : {});
export const startAccountLogin = (provider: string, signal?: AbortSignal) =>
  apiJson<{ readonly id: string }>(`/api/accounts/${encodeURIComponent(provider)}/login`, { method: "POST", ...(signal ? { signal } : {}) });
export const cancelAccountLogin = (id: string) =>
  apiVoid(`/api/accounts/logins/${encodeURIComponent(id)}/cancel`, { method: "POST" });
export const answerAccountLogin = (id: string, body: { readonly id: string; readonly value?: string; readonly confirmed?: boolean }) =>
  apiVoid(`/api/accounts/logins/${encodeURIComponent(id)}/response`, { method: "POST", body });
export const changeAccount = (provider: string, action: "remove" | "pin", name: string | null) =>
  apiVoid(`/api/accounts/manage/${encodeURIComponent(provider)}/${action}`, { method: "POST", body: { name } });

export function accountLoginEvents(id: string, receive: (event: LoginEvent) => void, fail: () => void): () => void {
  const source = new EventSource(`/api/accounts/logins/${encodeURIComponent(id)}/events`);
  source.onmessage = (event: MessageEvent<string>) => {
    let data: unknown;
    try { data = JSON.parse(event.data); } catch { fail(); return; }
    if (typeof data !== "object" || data === null || !("type" in data) || typeof data.type !== "string") {
      fail(); return;
    }
    // Parse the small, secret-free browser-facing event shape.
    const field = (key: string): string | undefined =>
      key in data && typeof Reflect.get(data, key) === "string" ? Reflect.get(data, key) : undefined;
    const options: unknown = "options" in data ? data.options : undefined;
    receive({
      type: data.type, url: field("url"), id: field("id"), method: field("method"),
      title: field("title"), message: field("message"), placeholder: field("placeholder"), error: field("error"),
      success: "success" in data && data.success === true,
      options: Array.isArray(options) ? options.filter((v): v is string => typeof v === "string") : undefined,
    });
  };
  source.onerror = () => { source.close(); fail(); };
  return () => source.close();
}

export function safeLoginURL(value: string): string | undefined {
  try {
    const url = new URL(value);
    if (["https:", "cursor:", "cursor-nightly:"].includes(url.protocol) ||
      (url.protocol === "http:" && ["localhost", "127.0.0.1"].includes(url.hostname))) return url.href;
  } catch { return undefined; }
  return undefined;
}
