import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import { ApiError } from "../lib/api";
import { saveAccountAPIKey, trimAnthropicAPIKey, validAnthropicAPIKey } from "../lib/accounts";
import type { AccountProvider, APIKeyResult } from "../lib/accounts";
import { useConfirm } from "./ConfirmDialog";

export function AccountAPIKeyForm({ provider, onCancel, onSaved, onBusy }: {
  readonly provider: AccountProvider;
  readonly onCancel: () => void;
  readonly onSaved: (result: APIKeyResult) => void;
  readonly onBusy: (busy: boolean) => void;
}) {
  const { t } = useT();
  const { confirm, dialog } = useConfirm(t);
  const [key, setKey] = useState("");
  const [shown, setShown] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  const save = async () => {
    if (saving) return;
    if (!validAnthropicAPIKey(key)) { setError(t("accounts.apiKeyInvalid")); return; }
    setSaving(true); onBusy(true); setError("");
    try {
      if (provider.authType === "oauth" && !await confirm({
        title: t("accounts.apiKeyReplaceTitle"), message: t("accounts.apiKeyReplaceConfirm"),
        confirmLabel: t("accounts.apiKeySave"), cancelLabel: t("accounts.apiKeyCancel"),
      })) return;
      if (!mounted.current) return;
      const result = await saveAccountAPIKey(trimAnthropicAPIKey(key));
      // Remove credentials before refreshing or displaying any completion UI.
      setKey(""); setShown(false);
      if (mounted.current) onSaved(result);
    } catch (cause) {
      if (mounted.current) {
        // Only server errors are user-facing; never render arbitrary thrown input.
        const message = cause instanceof ApiError ? cause.message.split(key).join("[redacted]").replace(/sk-ant-[^\s"<>]*/g, "[redacted]") : t("accounts.apiKeyFailed");
        setKey(""); setShown(false); setError(message);
      }
    } finally { if (mounted.current) setSaving(false); onBusy(false); }
  };
  return <>
    <form className="th-accounts-flow" onSubmit={(event) => { event.preventDefault(); void save(); }}>
      <label htmlFor="accounts-api-key">{t("accounts.provider.anthropic")}</label>
      <p id="accounts-api-key-help" className="th-accounts-hint">{t("accounts.apiKeyHelp")}</p>
      <p className="th-accounts-hint">{t("accounts.apiKeyBilling")}</p>
      <input id="accounts-api-key" className="th-input" type={shown ? "text" : "password"} autoComplete="off" spellCheck={false} autoCapitalize="off" aria-describedby="accounts-api-key-help" value={key} disabled={saving} onChange={(event) => setKey(event.target.value)} autoFocus />
      <div className="th-accounts-actions">
        <button type="button" className="th-btn th-btn--ghost" aria-pressed={shown} disabled={saving} onClick={() => setShown(!shown)}>{t(shown ? "accounts.apiKeyHide" : "accounts.apiKeyShow")}</button>
        <button type="submit" className="th-btn th-btn--primary" disabled={saving}>{t("accounts.apiKeySave")}</button>
        <button type="button" className="th-btn th-btn--ghost" disabled={saving} onClick={() => { setKey(""); onCancel(); }}>{t("accounts.apiKeyCancel")}</button>
      </div>
      {error && <p role="alert">{error}</p>}
    </form>
    {dialog}
  </>;
}
