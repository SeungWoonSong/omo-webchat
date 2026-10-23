import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { I18nContext } from "../../i18n";
import { Markdown } from "../../components/Markdown";
import { FileBrowser } from "../terminal/FileBrowser";
import { fsList, fsRead } from "../terminal/terminal";
import { ChatFilesContext, UserFileMessage, appendFileAttachments, splitFileAttachments, workspaceFilePath } from "./chatFiles";
import { ToolCard } from "./ToolCard";
import { i18n } from "./chatPaneTestHarness";
vi.mock("../terminal/terminal", () => ({ fsList: vi.fn(), uploadFiles: vi.fn(), fsRead: vi.fn(), downloadUrl: vi.fn() }));
let container: HTMLDivElement;
let root: ReturnType<typeof createRoot>;
beforeEach(() => { vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true); vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} }); container = document.createElement("div"); document.body.append(container); root = createRoot(container); });
afterEach(() => { act(() => root.unmount()); container.remove(); vi.unstubAllGlobals(); vi.resetAllMocks(); });
it("renders attachment-only prompts after the session dispatcher trims whitespace", () => {
  expect(splitFileAttachments(appendFileAttachments("", ["uploads/a.pdf"]).trim())).toEqual({ text: "", paths: ["uploads/a.pdf"] });
});
it("detects workspace file paths but excludes URLs, plain words, code, escapes and other roots", () => {
  for (const value of ["out/report.docx", "./x.md", "/work/out/a.png", "보고서 (1).pdf"]) expect(workspaceFilePath(value, "/work")).not.toBeNull();
  for (const value of ["https://x/a.pdf", "//x/a.pdf", "mailto:x.pdf", "hello", "const x = 1", "../a.pdf", "/work-other/a.pdf", "/etc/a.pdf", "a.pdf\n"]) expect(workspaceFilePath(value, "/work")).toBeNull();
});
it("renders markdown paths, user attachment chips and writing-tool open buttons without network calls", () => {
  const open = vi.fn();
  act(() => root.render(<I18nContext.Provider value={i18n}><ChatFilesContext.Provider value={{ root: "/work", open }}>
    <Markdown text={'[보고서](out/report.pdf) `./x.md` `/work/out/a.png` [web](https://example.com) `plain`\n\n```\nnot-a-link.pdf\n```'} />
    <UserFileMessage text={appendFileAttachments("check", ["uploads/a.docx"])} />
    <ToolCard toolCallId="write-1" toolName="write" phase="end" text="done" isError={false} args={{ path: "out/report.pdf" }} />
  </ChatFilesContext.Provider></I18nContext.Provider>));
  expect(container.querySelectorAll(".th-chat-file-link")).toHaveLength(5);
  expect(container.querySelector('a')?.href).toBe("https://example.com/");
  expect(container.textContent).not.toContain("[첨부 파일]");
  expect(fsList).not.toHaveBeenCalled();
  act(() => Array.from(container.querySelectorAll<HTMLButtonElement>("button")).find(button => button.textContent === "chat.fileOpen")!.click());
  expect(open).toHaveBeenCalledWith("out/report.pdf");
});
it("open-file request enters FileViewer, missing request emits toast", async () => {
  const notify = vi.fn();
  vi.mocked(fsList).mockResolvedValue({ path: "/work", parent: null, entries: [{ name: "a.docx", size: 10, isDir: false, modTime: "2026-10-23" }] });
  const render = (path: string, id: number) => root.render(<I18nContext.Provider value={i18n}><FileBrowser path="/work" wsId="w" tmId="c" requestedFile={{ path, id }} onClose={() => {}} notify={notify} width={320} onWidthChange={() => {}} /></I18nContext.Provider>);
  await act(async () => render("/work/a.docx", 1));
  expect(container.querySelector(".th-viewer")).not.toBeNull();
  expect(container.querySelector(".th-viewer-heading")?.textContent).toContain("a.docx");
  await act(async () => render("/work/missing.pdf", 2));
  expect(notify).toHaveBeenCalledWith("chat.fileMissing", "error");
});

it("keeps unsaved edits on cancelled transcript navigation, replaces only after discard", async () => {
  vi.mocked(fsList).mockResolvedValue({ path: "/work", parent: null, entries: ["a.md", "b.docx"].map(name => ({ name, size: 10, isDir: false, modTime: "2026-10-23" })) });
  vi.mocked(fsRead).mockResolvedValue({ content: "original", size: 8 });
  const render = (path: string, id: number) => root.render(<I18nContext.Provider value={i18n}><FileBrowser path="/work" wsId="w" tmId="c" requestedFile={{ path, id }} onClose={() => {}} notify={() => {}} width={320} onWidthChange={() => {}} /></I18nContext.Provider>);
  await act(async () => render("/work/a.md", 1));
  await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="viewer.edit"]')!.click());
  act(() => {
    const area = container.querySelector<HTMLTextAreaElement>("textarea")!;
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(area, "changed");
    area.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => render("/work/b.docx", 2));
  expect(document.querySelector(".th-confirm-title")?.textContent).toBe("editor.unsaved");
  await act(async () => document.querySelector<HTMLButtonElement>(".th-confirm .th-btn--ghost")!.click());
  expect(container.querySelector<HTMLTextAreaElement>("textarea")?.value).toBe("changed");
  await act(async () => render("/work/b.docx", 3));
  await act(async () => document.querySelector<HTMLButtonElement>(".th-confirm .th-btn--danger")!.click());
  expect(container.querySelector("textarea")).toBeNull();
  expect(container.querySelector(".th-viewer-heading")?.textContent).toContain("b.docx");
});
