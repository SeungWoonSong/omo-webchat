import { useEffect, useRef, useState } from "react";
import { useT } from "../../i18n";
import { apiDownload } from "../../lib/api";
import { Markdown } from "../../components/Markdown";
import { FileEditor } from "./FileEditor";
import type { FileEditorProps } from "./FileEditor";
import { downloadUrl, fsList, fsRead } from "./terminal";
import type { FsEntry } from "./terminal";
import { isTextPreview, markdownImageUrl, parseTable, previewKind, rawUrl } from "./filePreview";
import "../../styles/file-viewer.css";

interface FileViewerProps extends FileEditorProps {
  readonly expanded: boolean;
  readonly onExpand: () => void;
}

export function FileViewer({ path, name, requestClose, onClose, notify, expanded, onExpand }: FileViewerProps) {
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
  if (editing) return <FileEditor path={path} name={name} {...(requestClose ? { requestClose } : {})} onClose={target => {
    if (target === "browser") onClose(target); else setEditing(false);
  }} notify={notify} />;
  const url = rawUrl(path);
  let pretty = content ?? "";
  if (kind === "json" && content !== null) { try { pretty = JSON.stringify(JSON.parse(content), null, 2); } catch { /* Invalid JSON stays visible as source. */ } }
  return <section className="th-viewer" aria-label={t("viewer.title")}>
    <div className="th-viewer-toolbar">
      <strong title={path}>{name}</strong>
      <button type="button" className="th-btn" ref={backRef} onClick={() => onClose("editor")}>{t("viewer.back")}</button>
      <a className="th-btn" href={url} target="_blank" rel="noopener noreferrer">{t("viewer.newTab")}</a>
      <button type="button" className="th-btn" onClick={download}>{t("files.download")}</button>
      {(isTextPreview(kind) || name.toLowerCase().endsWith(".svg")) && <button type="button" className="th-btn" onClick={() => setEditing(true)}>{t("viewer.edit")}</button>}
      <button type="button" className="th-btn th-viewer-expand" aria-pressed={expanded} onClick={onExpand}>{t(expanded ? "viewer.shrink" : "viewer.expand")}</button>
      {kind === "table" && <button type="button" className="th-btn" aria-pressed={raw} onClick={() => setRaw(!raw)}>{t(raw ? "viewer.table" : "viewer.raw")}</button>}
    </div>
    <div className="th-viewer-body">
      {error ? <p role="alert">{error}</p> : kind === "markdown" ? content === null ? <p>{t("wizard.loading")}</p> :
        <div className="th-viewer-markdown"><Markdown text={content} imageUrl={src => markdownImageUrl(path, src)} /></div> :
      kind === "html" || kind === "pdf" ? <iframe title={name} src={url} {...(kind === "html" ? { sandbox: "allow-scripts allow-popups allow-forms" } : {})} /> :
      kind === "image" ? <button type="button" className={`th-viewer-image${zoom ? " th-viewer-image--zoom" : ""}`} aria-label={t(zoom ? "viewer.fit" : "viewer.zoom")} onClick={() => setZoom(!zoom)}><img src={url} alt={name} /></button> :
      kind === "audio" ? <audio controls src={url} aria-label={name} /> :
      kind === "video" ? <video controls src={url} aria-label={name} /> :
      kind === "unsupported" ? <div className="th-viewer-unsupported"><span aria-hidden="true">▤</span><h3>{name}</h3><p>{t("viewer.unsupported")}</p>
        {meta && <dl><dt>{t("viewer.size")}</dt><dd>{meta.size.toLocaleString(lang)} B</dd><dt>{t("viewer.modified")}</dt><dd>{new Date(meta.modTime).toLocaleString(lang)}</dd></dl>}
        <button type="button" className="th-btn" onClick={download}>{t("files.download")}</button></div> :
      content === null ? <p>{t("wizard.loading")}</p> : kind === "table" && !raw ? <><p>{t("viewer.rows", { n: 200 })}</p><div className="th-viewer-table"><table><tbody>{parseTable(content, name.toLowerCase().endsWith(".tsv") ? "\t" : ",").map((row, i) => <tr key={i}>{row.map((cell, j) => i === 0 ? <th key={j} scope="col">{cell}</th> : <td key={j}>{cell}</td>)}</tr>)}</tbody></table></div></> :
      <pre className="th-viewer-source">{pretty}</pre>}
    </div>
  </section>;
}
