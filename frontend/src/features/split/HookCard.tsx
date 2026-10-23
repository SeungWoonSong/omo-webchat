import { useState } from "react";
import { useT } from "../../i18n";
import { NoticeDisclosure } from "./NoticeDisclosure";

interface HookCardProps {
  readonly hookType: string;
  readonly text: string;
}

export function HookCard({ hookType, text }: HookCardProps) {
  const { t } = useT();
  const [open, setOpen] = useState(false);
  const preview = text.length > 80 ? `${text.slice(0, 80)}…` : text;

  if (hookType.startsWith("omo-model-profile:")) {
    return (
      <div data-hook-type={hookType}>
        <NoticeDisclosure title={t("notice.modelProfileTitle")}>
          <pre className="th-notice-full-text">{text}</pre>
        </NoticeDisclosure>
      </div>
    );
  }

  return (
    <div className="th-tool th-hook" data-hook-type={hookType}>
      <button
        type="button"
        className="th-tool-head"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <span className="th-tool-name">{hookType}</span>
        {!open && preview.length > 0 && <span className="th-tool-preview">{preview}</span>}
      </button>
      {open && text.length > 0 && <pre className="th-tool-body">{text}</pre>}
    </div>
  );
}
