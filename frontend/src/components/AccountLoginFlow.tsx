import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import type { useAccountLogin } from "./useAccountLogin";

export function AccountLoginFlow({ login }: { readonly login: ReturnType<typeof useAccountLogin> }) {
  const { t } = useT();
  const [value, setValue] = useState("");
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState(false);
  const heading = useRef<HTMLHeadingElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const complete = login.result === "accounts.success";
  const step = complete ? 3 : login.prompt?.method === "input" || login.prompt?.method === "confirm" ? 2 : 1;
  useEffect(() => { heading.current?.focus(); }, []);
  useEffect(() => { setValue(""); input.current?.focus(); }, [login.prompt?.id]);
  useEffect(() => { setCopied(false); setCopyError(false); }, [login.url]);
  return <section className="th-accounts-flow" aria-label={t("accounts.login")}>
    <h3 ref={heading} tabIndex={-1}>{t("accounts.login")}</h3>
    <ol className="th-accounts-steps" aria-label={t("accounts.steps")}>
      {["accounts.stepOpen", "accounts.stepVerify", "accounts.stepDone"].map((key, index) =>
        <li key={key} aria-current={step === index + 1 ? "step" : undefined} data-complete={step > index + 1}>
          <span aria-hidden="true">{index + 1}</span>{t(key)}
        </li>)}
    </ol>
    {login.busy && <>
      <p className="th-accounts-hint" role="status">{t(login.provider === "cursor" ? "accounts.cursorWait" : "accounts.wait")}</p>
      {login.url ? <div className="th-accounts-link">
        <a href={login.url} target="_blank" rel="noreferrer" className="th-btn th-btn--primary">{t("accounts.open")}</a>
        <button className="th-btn th-btn--ghost" onClick={async () => {
          try { await navigator.clipboard.writeText(login.url); setCopied(true); setCopyError(false); }
          catch { setCopyError(true); }
        }}>{t(copied ? "accounts.copied" : "accounts.copy")}</button>
        <input className="th-input th-accounts-secret" aria-label={t("accounts.url")} value={login.url} readOnly onFocus={(e) => e.currentTarget.select()} />
        {copyError && <p role="alert">{t("accounts.copyFailed")}</p>}
      </div> : !login.prompt && <p role="status">{t("accounts.preparing")}</p>}
      {login.notice && <p className="th-accounts-secret">{login.notice}</p>}
      {login.prompt && <div className="th-accounts-secret">
        <p>{login.prompt.title}</p>
        {login.prompt.method === "select" && <div className="th-accounts-actions">{login.prompt.options?.map((option) =>
          <button key={option} className="th-btn th-btn--ghost" disabled={login.sending} onClick={() => void login.answer({ value: option })}>{option}</button>)}</div>}
        {login.prompt.method === "confirm" && <div className="th-accounts-actions">
          <button className="th-btn th-btn--primary" disabled={login.sending} onClick={() => void login.answer({ confirmed: true })}>{t("accounts.yes")}</button>
          <button className="th-btn th-btn--ghost" disabled={login.sending} onClick={() => void login.answer({ confirmed: false })}>{t("accounts.no")}</button>
        </div>}
        {login.prompt.method === "input" && <form onSubmit={(e) => { e.preventDefault(); if (value.trim()) void login.answer({ value }); }}>
          <label htmlFor="accounts-code">{t("accounts.paste")}</label>
          <p id="accounts-code-help" className="th-accounts-hint">{t("accounts.pasteHelp")}</p>
          <input ref={input} id="accounts-code" className="th-input" aria-describedby="accounts-code-help" autoComplete="off" spellCheck={false} value={value} placeholder={login.prompt.placeholder} onChange={(e) => setValue(e.target.value)} />
          <button type="submit" className="th-btn th-btn--primary" disabled={!value.trim() || login.sending}>{t(login.sending ? "accounts.sending" : "accounts.send")}</button>
        </form>}
      </div>}
      <button className="th-btn th-btn--ghost" onClick={() => void login.cancel()}>{t("accounts.cancel")}</button>
    </>}
    {login.result && <p role="status" className={complete ? "th-accounts-success" : "th-accounts-hint"}>{t(login.result)}</p>}
    {login.error && <p role="alert">{t(login.error)}</p>}
  </section>;
}
