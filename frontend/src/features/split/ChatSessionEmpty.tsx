import { useT } from "../../i18n";

const SUGGESTIONS = ["organize", "draft", "summarize", "table"] as const;

export function ChatSessionEmpty({ onSuggestion }: { readonly onSuggestion: (text: string) => void }) {
  const { t } = useT();
  return <section className="th-chat-session-empty" aria-label={t("chat.empty.heading")}>
    <img src="/penguin.svg" alt="" width="48" height="48" />
    <h2>{t("chat.empty.heading")}</h2>
    <div className="th-chat-suggestions">
      {SUGGESTIONS.map(key => <button type="button" key={key} onClick={() => onSuggestion(t(`chat.empty.${key}`))}>{t(`chat.empty.${key}`)}</button>)}
    </div>
    <p>{t("chat.empty.hint")}</p>
  </section>;
}
