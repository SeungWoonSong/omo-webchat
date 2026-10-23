import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { useT } from "../../i18n";
import { apiDownload } from "../../lib/api";
import { Markdown } from "../../components/Markdown";
import { IconChevron, IconDownload, IconEdit, IconFile, IconGlobe, IconTerminal } from "../../components/icons";
import { FileEditor } from "./FileEditor";
import type { FileEditorProps } from "./FileEditor";
import { downloadUrl, fsList, fsRead } from "./terminal";
import type { FsEntry } from "./terminal";
import { isTextPreview, markdownImageUrl, parseTable, previewKind, rawUrl, workspaceRelativePath } from "./filePreview";
import "../../styles/file-viewer.css";

interface FileViewerProps extends FileEditorProps {
  readonly expanded: boolean;
  readonly onExpand: () => void;
  readonly workspaceRoot?: string;
  readonly closeButton?: ReactNode;
}

export function FileViewer({ path, name, requestClose, onClose, notify, expanded, onExpand, workspaceRoot = "", closeButton }: FileViewerProps) {
  const { t, lang } = useT();
  const kind = previewKind(name);
  const backRef = useRef<HTMLButtonElement>(null);
  const handledCloseRef = useRef<number>();
  const [editing, setEditing] = useState(false);
  const [content, setContent] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [meta, setMeta] = useState<FsEntry>();
  const [raw, setRaw] = useState(false);
  const [zoom, setZoom] = useState(false);
  useEffect(() => {
    let cancelled = false;
    setContent(null); setError("");
    if (isTextPreview(kind) && kind !== "html") {
      fsRead(path).then(file => { if (!cancelled) setContent(file.content); })
        .catch((err: unknown) => { if (!cancelled) setError(err instanceof Error ? err.message : t("files.error")); });
    }
    fsList(path.slice(0, path.lastIndexOf("/")) || "/").then(list => {
      if (!cancelled) setMeta(list.entries.find(entry => entry.name === name));
    }).catch(() => { /* Preview remains usable if metadata cannot be read. */ });
    return () => { cancelled = true; };
  }, [path, name, kind, t, editing]);
  useEffect(() => {
    if (!requestClose || handledCloseRef.current === requestClose.id) return;
    handledCloseRef.current = requestClose.id;
    if (!editing) onClose(requestClose.target);
  }, [requestClose, editing, onClose]);
  useEffect(() => { if (!editing) backRef.current?.focus(); }, [editing]);
  const download = (): void => {
    void apiDownload(downloadUrl(path), name).catch((err: unknown) => notify(err instanceof Error ? err.message : t("files.error"), "error"));
  };
  if (editing) return <>{closeButton && <div className="th-files-head"><span className="th-files-title">{t("files.title")}</span><span className="th-files-path">{workspaceRoot ? workspaceRelativePath(workspaceRoot, path.slice(0, path.lastIndexOf("/"))) : "./"}</span>{closeButton}</div>}<FileEditor path={path} name={name} {...(requestClose ? { requestClose } : {})} onClose={target => {
    if (target === "browser") onClose(target); else setEditing(false);
  }} notify={notify} /></>;
  const url = rawUrl(path);
  const directory = workspaceRoot ? workspaceRelativePath(workspaceRoot, path.slice(0, path.lastIndexOf("/"))) : "./";
  const FileIcon = kind === "html" ? IconGlobe : ["text", "json", "table"].includes(kind) ? IconTerminal : IconFile;
  let pretty = content ?? "";
  if (kind === "json" && content !== null) { try { pretty = JSON.stringify(JSON.parse(content), null, 2); } catch { /* Invalid JSON stays visible as source. */ } }
  return <section className="th-viewer" aria-label={t("viewer.title")}>
    <div className="th-viewer-toolbar">
      <button type="button" className="th-btn-icon th-viewer-back" ref={backRef} title={t("viewer.back")} aria-label={t("viewer.back")} onClick={() => onClose("editor")}><IconChevron size={16} /></button>
      <FileIcon size={18} className="th-viewer-file-icon" />
      <div className="th-viewer-heading"><strong title={name}>{name}</strong><span className="th-files-path" title={directory}><bdi dir="ltr">{directory}</bdi></span></div>
      <div className="th-viewer-actions">
        <a className="th-btn-icon" href={url} target="_blank" rel="noopener noreferrer" title={t("viewer.newTab")} aria-label={t("viewer.newTab")}><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true"><path d="M14 3h7v7M21 3 10 14M10 3H3v18h18v-7" /></svg></a>
        <button type="button" className="th-btn-icon" title={t("files.download")} aria-label={t("files.download")} onClick={download}><IconDownload size={16} /></button>
        {kind === "table" ? <button type="button" className="th-btn-icon" title={t("viewer.raw")} aria-label={t("viewer.raw")} aria-pressed={raw} onClick={() => setRaw(!raw)}><IconTerminal size={16} /></button> :
          (isTextPreview(kind) || name.toLowerCase().endsWith(".svg")) && <button type="button" className="th-btn-icon" title={t("viewer.edit")} aria-label={t("viewer.edit")} onClick={() => setEditing(true)}><IconEdit size={16} /></button>}
        <button type="button" className="th-btn-icon th-viewer-expand" title={t(expanded ? "viewer.shrink" : "viewer.expand")} aria-label={t(expanded ? "viewer.shrink" : "viewer.expand")} aria-pressed={expanded} onClick={onExpand}><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true"><path d={expanded ? "M3 9h6V3m6 0v6h6M3 15h6v6m6 0v-6h6" : "M9 3H3v6m12-6h6v6M3 15v6h6m6 0h6v-6"} /></svg></button>
        {closeButton}
      </div>
    </div>
    <div className={`th-viewer-body${kind === "html" || kind === "pdf" ? " th-viewer-body--frame" : ""}${kind === "image" ? " th-viewer-body--image" : ""}`}>
      {error ? <p role="alert">{error}</p> : kind === "markdown" ? content === null ? <p>{t("wizard.loading")}</p> :
        <div className="th-viewer-markdown th-chat-markdown"><Markdown text={content} imageUrl={src => markdownImageUrl(path, src)} /></div> :
      kind === "html" || kind === "pdf" ? <iframe title={name} src={kind === "pdf" ? `${url}#view=FitH&navpanes=0` : url} {...(kind === "html" ? { sandbox: "allow-scripts allow-popups allow-forms" } : {})} /> :
      kind === "image" ? <button type="button" className={`th-viewer-image${zoom ? " th-viewer-image--zoom" : ""}`} aria-label={t(zoom ? "viewer.fit" : "viewer.zoom")} aria-pressed={zoom} onClick={() => setZoom(!zoom)}><img src={url} alt={name} /></button> :
      kind === "audio" ? <audio controls src={url} aria-label={name} /> :
      kind === "video" ? <video controls src={url} aria-label={name} /> :
      kind === "unsupported" ? <div className="th-viewer-unsupported"><IconFile size={40} /><h3>{name}</h3><p>{t("viewer.unsupported")}</p>
        {meta && <dl><dt>{t("viewer.size")}</dt><dd>{meta.size.toLocaleString(lang)} B</dd><dt>{t("viewer.modified")}</dt><dd>{new Date(meta.modTime).toLocaleString(lang)}</dd></dl>}
        <button type="button" className="th-btn th-btn--primary" onClick={download}><IconDownload size={16} />{t("files.download")}</button></div> :
      content === null ? <p>{t("wizard.loading")}</p> : kind === "table" && !raw ? <><p className="th-viewer-table-hint">{t("viewer.rows", { n: 200 })}</p><div className="th-viewer-table"><table><tbody>{parseTable(content, name.toLowerCase().endsWith(".tsv") ? "\t" : ",").map((row, i) => <tr key={i}>{row.map((cell, j) => i === 0 ? <th key={j} scope="col">{cell}</th> : <td key={j}>{cell}</td>)}</tr>)}</tbody></table></div></> :
      <pre className="th-viewer-source">{pretty}</pre>}
    </div>
  </section>;
}
