import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import { ModalDialog } from "./ModalDialog";
import { useConfirm } from "./ConfirmDialog";
import { AccountAPIKeyForm } from "./AccountAPIKeyForm";
import { AccountLoginFlow } from "./AccountLoginFlow";
import { useAccountLogin } from "./useAccountLogin";
import { changeAccount, listAccountProviders } from "../lib/accounts";
import type { AccountProvider } from "../lib/accounts";
import "../styles/accounts.css";

export function AccountsPanel({ onClose }: { readonly onClose: () => void }) {
  const { t } = useT();
  const { confirm, dialog } = useConfirm(t);
  const [providers, setProviders] = useState<readonly AccountProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [changing, setChanging] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState("");
  const [keyForm, setKeyForm] = useState(false);
  const [keyResult, setKeyResult] = useState("");
  const mounted = useRef(true);
  const refresh = async (signal?: AbortSignal) => {
    setLoading(true);
    try {
      const data = await listAccountProviders(signal);
      if (mounted.current) { setProviders(data.providers); setError(""); }
    } catch {
      if (mounted.current && !signal?.aborted) setError("accounts.loadFailed");
    } finally { if (mounted.current) setLoading(false); }
  };
  const login = useAccountLogin(() => { void refresh(); });
  const busy = login.busy || changing;
  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void refresh(controller.signal);
    return () => { mounted.current = false; controller.abort(); };
  }, []);

  const manage = async (p: AccountProvider, action: "remove" | "pin", name: string | null) => {
    setChanging(true);
    try {
      if (action === "remove") {
        const account = p.accounts.find((a) => a.name === name);
        const label = account?.displayName ? `${account.displayName} (${name})` : name;
        if (!await confirm({
          title: t("accounts.removeTitle"),
          message: t("accounts.removeMessage", { account: label ?? "", provider: t(`accounts.provider.${p.id}`) }),
          confirmLabel: t("accounts.remove"), cancelLabel: t("accounts.keep"), danger: true,
        })) return;
      }
      if (!mounted.current) return;
      setError(""); setResult("");
      await changeAccount(p.id, action, name);
      if (mounted.current) { setResult(action === "remove" ? "accounts.removed" : "accounts.pinSaved"); await refresh(); }
    } catch {
      if (mounted.current) setError("accounts.manageFailed");
    } finally { if (mounted.current) setChanging(false); }
  };

  return <>
    <ModalDialog open onClose={onClose} labelledBy="accounts-title" closeLabel={t("accounts.close")}>
      <div className="th-accounts">
        <div className="th-accounts-title-row">
          <h2 id="accounts-title">{t("accounts.title")}</h2>
          <button className="th-btn-icon th-accounts-refresh" title={t("accounts.refresh")} aria-label={t("accounts.refresh")} disabled={loading || busy} onClick={() => void refresh()}>↻</button>
        </div>
        <div className="th-accounts-body">
        <header className="th-accounts-header">
          <p className="th-accounts-hint">{t("accounts.description")}</p>
        </header>
        <div className="th-accounts-feedback" aria-live="polite">
          {loading && <p role="status">{t("accounts.loading")}</p>}
          {error && <p role="alert">{t(error)}</p>}
          {result && <p role="status">{t(result)}</p>}
          {keyResult && <p role="status">{keyResult}</p>}
        </div>
        {!loading && providers.length === 0 && <p>{t("accounts.emptyProviders")}</p>}
        <ul className="th-accounts-list" aria-busy={loading}>
          {providers.map((p) => {
            const selected = p.id === login.provider;
            const status = selected && login.busy ? "progress" : selected && login.error ? "error" : p.status.configured ? "connected" : "notConnected";
            return <li key={p.id} data-provider={p.id}>
              <div className="th-accounts-provider">
                <div className="th-accounts-provider-title"><h3>{t(`accounts.provider.${p.id}`)}</h3><span className="th-accounts-badge" data-status={status}>{t(`accounts.${status}`)}</span></div>
                <button className="th-btn th-btn--primary" disabled={busy || loading || p.id === "anthropic" && keyForm} onClick={() => { setResult(""); setKeyResult(""); if (p.id === "anthropic") setKeyForm(true); else void login.start(p.id); }}>{t(p.id === "anthropic" ? "accounts.apiKeyAdd" : "accounts.add")}</button>
              </div>
              {p.id === "anthropic" && p.keyHint && <p className="th-accounts-hint">{t("accounts.apiKeyDefault", { hint: `…${p.keyHint}` })}</p>}
              {p.id === "anthropic" && keyForm && <AccountAPIKeyForm provider={p} onBusy={setChanging} onCancel={() => setKeyForm(false)} onSaved={(saved) => {
                setKeyForm(false);
                setKeyResult(`${t("accounts.apiKeySaved")}${saved.keyHint ? ` (…${saved.keyHint})` : ""}${saved.replaced === "oauth" ? ` · ${t("accounts.apiKeyReplaced")}` : ""}`);
                void refresh();
              }} />}
              {p.accounts.length === 0 ? <p className="th-accounts-hint">{t("accounts.empty")}</p> :
                <ul className="th-accounts-saved">
                  {p.accounts.map((a) => <li key={a.name}>
                    <div className="th-accounts-identity">
                      <strong>{a.displayName ? `${a.displayName} (${a.name})` : a.name}{p.id === "anthropic" && p.accounts.length === 1 && p.keyHint && <> · <span className="th-accounts-key-hint">…{p.keyHint}</span></>}</strong>
                      <span className="th-accounts-hint">{t(a.blocked ? "accounts.blocked" : "accounts.available")}{a.pinned ? ` · ${t("accounts.pinned")}` : ""}</span>
                    </div>
                    <div className="th-accounts-actions">
                      <button className="th-btn th-btn--ghost" disabled={busy || loading || a.blocked && !a.pinned} onClick={() => void manage(p, "pin", a.pinned ? null : a.name)}>{t(a.pinned ? "accounts.unpin" : "accounts.pin")}</button>
                      {a.source === "env" || a.source === "environment" ? <span className="th-accounts-hint">{t("accounts.environment")}</span> :
                        <button className="th-btn th-btn--ghost th-accounts-remove" disabled={busy || loading} onClick={() => void manage(p, "remove", a.name)}>{t("accounts.remove")}</button>}
                    </div>
                  </li>)}
                </ul>}
              {selected && !result && <AccountLoginFlow login={login} />}
            </li>;
          })}
        </ul>
        <p className="th-accounts-hint">{t("accounts.storageHint")}</p>
        </div>
      </div>
    </ModalDialog>
    {dialog}
  </>;
}
