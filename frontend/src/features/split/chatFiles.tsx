import { createContext, useContext } from "react";
import type { ReactNode } from "react";
import { IconFile } from "../../components/icons";
import { useT } from "../../i18n";
import "../../styles/chat-files.css";

export const ChatFilesContext = createContext<{ readonly root: string; readonly open: (path: string) => void } | null>(null);

const DOCUMENT_MEDIA_EXTENSIONS = new Set("pdf docx doc xlsx xls csv tsv pptx ppt hwp hwpx md txt html htm json png jpg jpeg gif webp svg mp3 mp4 wav zip".split(" "));
const FILE_EXTENSIONS = new Set([...DOCUMENT_MEDIA_EXTENSIONS, ..."py ts tsx js go sh yaml yml".split(" ")]);

/** Pure lexical detection; existence/permissions are checked only on click. */
export function workspaceFilePath(value: string, root: string, requireExtension = true): string | null {
  if (!value || value !== value.trim() || /[\x00-\x1f\x7f\\]/.test(value) || /^(?:[a-z][a-z\d+.-]*:|\/\/)/i.test(value)) return null;
  const base = root.replace(/\/+$/, "");
  let path = value;
  if (path.startsWith("/")) {
    if (!base || !path.startsWith(`${base}/`)) return null;
    path = path.slice(base.length + 1);
  }
  const parts: string[] = [];
  for (const part of path.split("/")) {
    if (part === "..") { if (!parts.length) return null; parts.pop(); }
    else if (part && part !== ".") parts.push(part);
  }
  const name = parts.at(-1);
  if (!name) return null;
  if (requireExtension) {
    const extension = name.slice(name.lastIndexOf(".") + 1).toLowerCase();
    const allowed = value.includes("/") ? FILE_EXTENSIONS : DOCUMENT_MEDIA_EXTENSIONS;
    if (!name.includes(".") || !allowed.has(extension)) return null;
  }
  return parts.join("/");
}

export function ChatFileLink({ path, children }: { readonly path: string; readonly children?: ReactNode }) {
  const files = useContext(ChatFilesContext);
  const { t } = useT();
  if (!files) return <>{children ?? path}</>;
  return <button type="button" className="th-chat-file-link" title={path} aria-label={t("chat.openFile", { name: path })} onClick={() => files.open(path)}><IconFile size={14} /><span>{children ?? path}</span></button>;
}

// Stable wire format, independent of locale. See FORK-NOTES.md.
export const ATTACHMENT_MARKER = "\n\n[첨부 파일]\n";
export function appendFileAttachments(text: string, paths: readonly string[]): string {
  return paths.length ? text + ATTACHMENT_MARKER + paths.map(path => `- ${path}`).join("\n") : text;
}
export function splitFileAttachments(text: string): { text: string; paths: string[] } {
  // Session dispatch trims an attachment-only draft's leading whitespace.
  const standalone = text.startsWith("[첨부 파일]\n");
  const at = standalone ? 0 : text.lastIndexOf(ATTACHMENT_MARKER);
  if (at < 0) return { text, paths: [] };
  const lines = text.slice(at + (standalone ? "[첨부 파일]\n".length : ATTACHMENT_MARKER.length)).split("\n");
  if (!lines.length || lines.some(line => !line.startsWith("- uploads/") || !workspaceFilePath(line.slice(2), "", false))) return { text, paths: [] };
  return { text: text.slice(0, at), paths: lines.map(line => line.slice(2)) };
}
export function UserFileMessage({ text, renderText }: { readonly text: string; readonly renderText?: (text: string) => ReactNode }) {
  const parsed = splitFileAttachments(text);
  return <>{renderText ? renderText(parsed.text) : <span>{parsed.text}</span>}{parsed.paths.length > 0 && <span className="th-chat-file-chips">{parsed.paths.map(path => <ChatFileLink key={path} path={path} />)}</span>}</>;
}
