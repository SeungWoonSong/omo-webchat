import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { FileViewer } from "./FileViewer";
import { I18nContext } from "../../i18n";
import { i18n } from "./fileBrowserTestSupport";
import { apiDownload } from "../../lib/api";
import { markdownImageUrl, parseTable, previewKind } from "./filePreview";

vi.mock("./terminal", () => ({ fsList: vi.fn().mockResolvedValue({ entries: [{ name: "fake.docx", size: 15, modTime: "2026-10-23T00:00:00Z" }] }), fsRead: vi.fn().mockResolvedValue({ content: "hello", size: 5 }), fsWrite: vi.fn(), downloadUrl: (path: string) => `/api/fs/download?path=${encodeURIComponent(path)}` }));
vi.mock("../../lib/api", async importOriginal => ({ ...await importOriginal<typeof import("../../lib/api")>(), apiDownload: vi.fn().mockResolvedValue(undefined) }));
afterEach(() => { vi.unstubAllGlobals(); vi.clearAllMocks(); });

it.each([["README.md", "markdown"], ["a.HTML", "html"], ["a.svg", "image"], ["a.png", "image"], ["a.jpg", "image"], ["a.pdf", "pdf"], ["a.mp3", "audio"], ["a.mp4", "video"], ["a.csv", "table"], ["a.tsv", "table"], ["a.json", "json"], ["a.py", "text"], ["a.docx", "unsupported"], ["a.hwp", "unsupported"], ["a.zip", "unsupported"]])("selects %s preview", (name, kind) => expect(previewKind(name!)).toBe(kind));
it("resolves relative Markdown images without rewriting remote images", () => {
  expect(markdownImageUrl("/work/docs/readme.md", "../image.png")).toBe("/api/fs/raw?path=%2Fwork%2Fimage.png");
  expect(markdownImageUrl("/work/a #/readme.md", "image%20%23.png")).toBe("/api/fs/raw?path=%2Fwork%2Fa+%23%2Fimage+%23.png");
  expect(markdownImageUrl("/work/readme.md", "https://example.org/i.png")).toBe("https://example.org/i.png");
});
it("parses quoted CSV and TSV, bounded to 200 rows", () => {
  expect(parseTable('a,b\r\n"hello, world","a""b"\n"multi\nline",x', ",")).toEqual([["a", "b"], ["hello, world", 'a"b'], ["multi\nline", "x"]]);
  expect(parseTable("a\tb\n1\t2", "\t")).toEqual([["a", "b"], ["1", "2"]]);
  expect(parseTable("x\n".repeat(300), ",")).toHaveLength(200);
  expect(parseTable("x,".repeat(1000), ",")[0]).toHaveLength(100);
});
it("toolbar opens raw, downloads, expands, closes and enters the existing editor only explicitly", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("div"); document.body.appendChild(host); const root = createRoot(host);
  const close = vi.fn(); const expand = vi.fn();
  try {
    await act(async () => root.render(<I18nContext.Provider value={i18n}><FileViewer name="file.py" path="/work/file.py" onClose={close} notify={vi.fn()} expanded={false} onExpand={expand} /></I18nContext.Provider>));
    const button = (label: string) => [...host.querySelectorAll("button")].find(el => el.textContent === label)!;
    expect(host.querySelector("textarea")).toBeNull();
    expect(host.querySelector("pre")?.textContent).toBe("hello");
    const link = host.querySelector("a")!;
    expect(link.getAttribute("href")).toBe("/api/fs/raw?path=%2Fwork%2Ffile.py"); expect(link.target).toBe("_blank"); expect(link.rel).toContain("noopener");
    await act(async () => button("Download").click()); expect(apiDownload).toHaveBeenCalledWith("/api/fs/download?path=%2Fwork%2Ffile.py", "file.py");
    act(() => button("Expand").click()); expect(expand).toHaveBeenCalledOnce();
    act(() => button("Back / Close").click()); expect(close).toHaveBeenCalledWith("editor");
    await act(async () => button("Edit").click()); expect(host.querySelector("textarea")?.value).toBe("hello");
  } finally { act(() => root.unmount()); host.remove(); }
});

it("sandboxes HTML without same-origin and shows unsupported metadata/download", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("div"); document.body.appendChild(host); const root = createRoot(host);
  const render = (name: string) => act(async () => root.render(<I18nContext.Provider value={i18n}><FileViewer key={name} name={name} path={`/work/${name}`} onClose={vi.fn()} notify={vi.fn()} expanded={false} onExpand={vi.fn()} /></I18nContext.Provider>));
  try {
    await render("page.html"); expect(host.querySelector("iframe")?.getAttribute("sandbox")).toBe("allow-scripts allow-popups allow-forms");
    await render("fake.docx"); expect(host.textContent).toContain("Preview unavailable — download"); expect(host.textContent).toContain("15 B"); expect(host.querySelector("textarea")).toBeNull();
  } finally { act(() => root.unmount()); host.remove(); }
});
