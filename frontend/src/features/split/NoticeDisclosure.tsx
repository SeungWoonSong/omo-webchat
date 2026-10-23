import { useState } from "react";
import type { ReactNode } from "react";
import { useNoticePreference } from "../../app-config";
import { IconChevron, IconMessage } from "../../components/icons";

interface NoticeDisclosureProps {
  readonly title: string;
  readonly children: ReactNode;
  readonly disabled?: boolean;
}

/** Only informational content reaches this component; diagnostics bypass it. */
export function NoticeDisclosure({ title, children, disabled = false }: NoticeDisclosureProps) {
  const { collapseSystemNotices } = useNoticePreference();
  const [open, setOpen] = useState(false);
  if (!collapseSystemNotices || disabled) return <>{children}</>;
  return (
    <div className="th-notice-disclosure">
      <button
        type="button"
        className="th-notice-disclosure-toggle"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <IconMessage size={14} />
        <span>{title}</span>
        <IconChevron size={12} />
      </button>
      {open && children}
    </div>
  );
}
