import { useEffect, useRef, useState } from "react";
import { accountLoginEvents, answerAccountLogin, cancelAccountLogin, safeLoginURL, startAccountLogin } from "../lib/accounts";
import type { LoginEvent } from "../lib/accounts";

export function useAccountLogin(onSuccess: () => void) {
  const [provider, setProvider] = useState("");
  const [busy, setBusy] = useState(false);
  const [url, setURL] = useState("");
  const [prompt, setPrompt] = useState<LoginEvent>();
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [result, setResult] = useState("");
  const [sending, setSending] = useState(false);
  const active = useRef<{ id: string; stop: () => void }>();
  const generation = useRef(0);
  const success = useRef(onSuccess);
  success.current = onSuccess;

  useEffect(() => () => {
    generation.current++;
    active.current?.stop();
  }, []);

  const finish = () => {
    active.current?.stop();
    active.current = undefined;
    setBusy(false); setSending(false); setURL(""); setPrompt(undefined); setNotice("");
  };

  const start = async (id: string) => {
    const current = ++generation.current;
    setProvider(id); setBusy(true); setURL(""); setPrompt(undefined); setNotice(""); setError(""); setResult("");
    try {
      const flow = await startAccountLogin(id);
      if (current !== generation.current) {
        await cancelAccountLogin(flow.id);
        return;
      }
      const stop = accountLoginEvents(flow.id, (event) => {
        if (current !== generation.current) return;
        switch (event.type) {
          case "auth_login_url": {
            const nextURL = safeLoginURL(event.url ?? "");
            if (nextURL) setURL(nextURL);
            else setError("accounts.invalidURL");
            break;
          }
          case "extension_ui_request":
            if (event.method === "notify") setNotice(event.message ?? event.title ?? "");
            else setPrompt(event);
            break;
          case "auth_login_end":
            finish();
            if (!event.success && event.error) setError("accounts.loginFailed");
            else setResult(event.success ? "accounts.success" : "accounts.ended");
            if (event.success) success.current();
            break;
          default:
            break; // Other engine events do not belong to the login UI.
        }
      }, () => {
        if (current !== generation.current) return;
        finish(); setError("accounts.disconnected");
      });
      active.current = { id: flow.id, stop };
    } catch {
      if (current === generation.current) { finish(); setError("accounts.failed"); }
    }
  };

  const answer = async (response: { readonly value?: string; readonly confirmed?: boolean }) => {
    const flow = active.current;
    const pending = prompt;
    if (!flow || !pending?.id || sending) return;
    const current = generation.current;
    setSending(true); setError("");
    try {
      await answerAccountLogin(flow.id, { id: pending.id, ...response });
      if (current === generation.current) setPrompt((next) => next?.id === pending.id ? undefined : next);
    } catch {
      if (current === generation.current) setError("accounts.failed");
    } finally {
      if (current === generation.current) setSending(false);
    }
  };

  const cancel = async () => {
    const flow = active.current;
    if (!flow) {
      generation.current++;
      finish(); setResult("accounts.ended");
      return;
    }
    const current = generation.current;
    try {
      await cancelAccountLogin(flow.id);
      if (current === generation.current) {
        generation.current++;
        finish(); setError(""); setResult("accounts.ended");
      }
    } catch {
      if (current === generation.current) setError("accounts.cancelFailed");
    }
  };

  return { provider, busy, url, prompt, notice, error, result, sending, start, answer, cancel };
}
