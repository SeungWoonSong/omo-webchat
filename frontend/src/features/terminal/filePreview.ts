import { qs } from "../../lib/api";

export type PreviewKind = "markdown" | "html" | "image" | "pdf" | "audio" | "video" | "table" | "json" | "text" | "office" | "unsupported";
export function previewKind(name: string): PreviewKind {
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  if (["md", "markdown"].includes(ext)) return "markdown";
  if (["html", "htm", "xhtml"].includes(ext)) return "html";
  if (["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg"].includes(ext)) return "image";
  if (["docx", "doc", "odt", "rtf", "xlsx", "xls", "ods", "pptx", "ppt", "odp", "hwp"].includes(ext)) return "office";
  if (ext === "pdf") return "pdf";
  if (["mp3", "wav", "ogg", "m4a", "flac"].includes(ext)) return "audio";
  if (["mp4", "webm", "mov", "ogv"].includes(ext)) return "video";
  if (["csv", "tsv"].includes(ext)) return "table";
  if (ext === "json") return "json";
  if (["txt", "py", "js", "ts", "tsx", "jsx", "go", "rs", "css", "scss", "sh", "bash", "zsh", "yaml", "yml", "toml", "xml", "log", "ini", "conf", "sql", "c", "h", "cpp", "java", "rb", "env"].includes(ext) || !name.includes(".") || name.startsWith(".env")) return "text";
  return "unsupported";
}
export function isTextPreview(kind: PreviewKind): boolean {
  return ["markdown", "html", "table", "json", "text"].includes(kind);
}
export function previewUrl(path: string): string { return `/api/fs/preview${qs({ path })}`; }
export function rawUrl(path: string): string { return `/api/fs/raw${qs({ path })}`; }
export function markdownImageUrl(path: string, src: string): string {
  if (/^(?:[a-z][a-z\d+.-]*:|\/\/|#)/i.test(src)) return src;
  const dir = path.slice(0, path.lastIndexOf("/") + 1);
  try {
    const base = new URL("https://workspace.invalid");
    base.pathname = dir;
    return rawUrl(decodeURIComponent(new URL(src, base).pathname));
  }
  catch { return rawUrl(dir + src); }
}

// Bounded RFC4180-style parsing, including quoted delimiters/newlines and "" escapes.
export function parseTable(text: string, delimiter: string, limit = 200): string[][] {
  const rows: string[][] = []; let row: string[] = []; let cell = ""; let quoted = false;
  for (let i = 0; i < text.length && rows.length < limit; i++) {
    const ch = text[i];
    if (ch === '"') {
      if (quoted && text[i + 1] === '"') { cell += '"'; i++; }
      else quoted = !quoted;
    } else if (!quoted && ch === delimiter) { if (row.length < 100) row.push(cell); cell = ""; }
    else if (!quoted && (ch === "\n" || ch === "\r")) {
      if (ch === "\r" && text[i + 1] === "\n") i++;
      rows.push(row.length < 100 ? [...row, cell] : row); row = []; cell = "";
    } else cell += ch;
  }
  if (rows.length < limit && (cell || row.length)) rows.push(row.length < 100 ? [...row, cell] : row);
  return rows;
}

// Never expose an absolute server path, even if a stale listing is outside the workspace.
export function workspaceRelativePath(root: string, path: string): string {
  const base = root.replace(/\/+$/, "");
  return path === base ? "./" : path.startsWith(`${base}/`) ? path.slice(base.length + 1) : "./";
}
