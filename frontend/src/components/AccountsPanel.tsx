import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import { ModalDialog } from "./ModalDialog";
import { accountLoginEvents, answerAccountLogin, cancelAccountLogin, listAccountProviders, safeLoginURL, startAccountLogin } from "../lib/accounts";
import type { AccountProvider, LoginEvent } from "../lib/accounts";
import "../styles/accounts.css";

export function AccountsPanel({ onClose }: { readonly onClose: () => void }) {
  const { t } = useT();
  const [providers, setProviders] = useState<readonly AccountProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [provider, setProvider] = useState("");
  const [url, setURL] = useState("");
  const [prompt, setPrompt] = useState<LoginEvent>();
  const [notice, setNotice] = useState("");
  const [value, setValue] = useState("");
  const [error, setError] = useState("");
  const [result, setResult] = useState("");
  const [copied, setCopied] = useState(false);
  const active = useRef<{ id: string; stop: () => void }>();
  const mounted = useRef(true);
  const popup = useRef<Window | null>(null);

  const refresh = async (signal?: AbortSignal) => {
    setLoading(true);
    try {
      const data = await listAccountProviders(signal);
      if (mounted.current) { setProviders(data.providers); setError(""); }
    } catch (err) {
      if (mounted.current && !signal?.aborted) setError(err instanceof Error ? err.message : t("accounts.failed"));
    } finally { if (mounted.current) setLoading(false); }
  };

  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void refresh(controller.signal);
    return () => {
      mounted.current = false;
      controller.abort();
      active.current?.stop(); // Stream disconnect cancels server-side.
      popup.current?.close();
    };
  }, []);

  const start = async (id: string) => {
    setBusy(true); setProvider(id); setURL(""); setPrompt(undefined); setNotice(""); setValue(""); setError(""); setResult(""); setCopied(false);
    // Reserve a tab during the user gesture; later OAuth events cannot open popups.
    popup.current = window.open("about:blank", "_blank");
    if (popup.current) popup.current.opener = null;
    try {
      const flow = await startAccountLogin(id);
      if (!mounted.current) { await cancelAccountLogin(flow.id); return; }
      const stop = accountLoginEvents(flow.id, (event) => {
        switch (event.type) {
          case "auth_login_url": {
            const nextURL = safeLoginURL(event.url ?? "");
            if (nextURL) {
              setURL(nextURL);
              if (popup.current) popup.current.location.href = nextURL;
            }
            break;
          }
          case "extension_ui_request":
            if (event.method === "notify") setNotice(event.message ?? event.title ?? "");
            else { setPrompt(event); setValue(""); }
            break;
          case "auth_login_end":
            active.current?.stop(); active.current = undefined; setBusy(false); setPrompt(undefined); setURL(""); setNotice("");
            popup.current?.close(); popup.current = null;
            setResult(event.success ? "accounts.success" : "accounts.ended");
            void refresh();
            break;
        }
      }, () => {
        active.current?.stop(); active.current = undefined;
        setBusy(false); setURL(""); setPrompt(undefined); setNotice("");
        popup.current?.close(); popup.current = null;
        setError(t("accounts.disconnected"));
      });
      active.current = { id: flow.id, stop };
    } catch (err) {
      if (mounted.current) { setError(err instanceof Error ? err.message : t("accounts.failed")); setBusy(false); }
      popup.current?.close(); popup.current = null;
    }
  };

  const answer = async (confirmed?: boolean, selected?: string) => {
    const flow = active.current;
    if (!flow || !prompt?.id) return;
    try {
      await answerAccountLogin(flow.id, { id: prompt.id, ...(confirmed === undefined ? { value: selected ?? value } : { confirmed }) });
      setValue(""); setPrompt(undefined);
    } catch (err) { setError(err instanceof Error ? err.message : t("accounts.failed")); }
  };

  return (
    <ModalDialog open onClose={onClose} labelledBy="accounts-title" closeLabel={t("accounts.close")}>
      <div className="th-accounts">
        <h2 id="accounts-title">{t("accounts.title")}</h2>
        <p className="th-accounts-hint">{t("accounts.description")}</p>
        <button className="th-btn th-btn--ghost" disabled={loading || busy} onClick={() => void refresh()}>{t("accounts.refresh")}</button>
        {loading && <p role="status">{t("accounts.loading")}</p>}
        {error && <p role="alert">{error}</p>}
        {result && <p role="status">{t(result)}</p>}
        <ul className="th-accounts-list">
          {providers.map((p) => (
            <li key={p.id} data-provider={p.id}>
              <strong>{p.name}</strong>
              <span className="th-accounts-hint">{t(p.status.configured ? "accounts.connected" : "accounts.notConnected")}</span>
              {p.accounts.map((a) => <span key={a.name}>{a.displayName ? `${a.displayName} (${a.name})` : a.name}{a.blocked ? ` · ${t("accounts.blocked")}` : ""}</span>)}
              <button className="th-btn th-btn--ghost" disabled={busy} onClick={() => void start(p.id)}>{t("accounts.add")}</button>
            </li>
          ))}
        </ul>
        {busy && (
          <section className="th-accounts-flow" aria-label={t("accounts.login")}>
            <h3>{providers.find((p) => p.id === provider)?.name}</h3>
            <p role="status">{t(provider === "cursor" ? "accounts.cursorWait" : "accounts.wait")}</p>
            {url && <>
              <a href={url} target="_blank" rel="noreferrer" className="th-btn th-btn--primary">{t("accounts.open")}</a>
              <input className="th-input th-accounts-secret" aria-label={t("accounts.url")} value={url} readOnly />
              <button className="th-btn th-btn--ghost" onClick={async () => {
                try { await navigator.clipboard.writeText(url); setCopied(true); }
                catch { setError(t("accounts.copyFailed")); }
              }}>{t(copied ? "accounts.copied" : "accounts.copy")}</button>
            </>}
            {notice && <p className="th-accounts-secret">{notice}</p>}
            {prompt && <div className="th-accounts-secret">
              <p>{prompt.title}</p>
              {prompt.method === "select" && prompt.options?.map((option) => <button key={option} className="th-btn th-btn--ghost" onClick={() => void answer(undefined, option)}>{option}</button>)}
              {prompt.method === "confirm" && <>
                <button className="th-btn th-btn--primary" onClick={() => void answer(true)}>{t("accounts.yes")}</button>
                <button className="th-btn th-btn--ghost" onClick={() => void answer(false)}>{t("accounts.no")}</button>
              </>}
              {prompt.method === "input" && <form onSubmit={(e) => { e.preventDefault(); void answer(); }}>
                <label htmlFor="accounts-code">{t("accounts.paste")}</label>
                <input id="accounts-code" className="th-input" autoComplete="off" spellCheck={false} value={value} placeholder={prompt.placeholder} onChange={(e) => setValue(e.target.value)} />
                <button type="submit" className="th-btn th-btn--primary" disabled={!value.trim()}>{t("accounts.send")}</button>
              </form>}
            </div>}
            <button className="th-btn th-btn--ghost" onClick={() => {
              if (active.current) void cancelAccountLogin(active.current.id).catch((err: unknown) => setError(err instanceof Error ? err.message : t("accounts.failed")));
            }}>{t("accounts.cancel")}</button>
          </section>
        )}
      </div>
    </ModalDialog>
  );
}
