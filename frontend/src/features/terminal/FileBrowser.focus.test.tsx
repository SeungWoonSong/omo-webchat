import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act } from "react";
import { createRoot } from "react-dom/client";
import type { Root } from "react-dom/client";
import { I18nContext } from "../../i18n";
import { FileBrowser } from "./FileBrowser";
import { fsList, fsRead, fsWrite } from "./terminal";
import { i18n, listing } from "./fileBrowserTestSupport";

vi.mock("./terminal", () => ({
  downloadUrl: (path: string) => `/download?path=${path}`,
  fsList: vi.fn(),
  fsRead: vi.fn(),
  fsWrite: vi.fn(),
  uploadFiles: vi.fn(),
}));

describe("FileBrowser focus", () => {
  let container: HTMLDivElement;
  let opener: HTMLButtonElement;
  let root: Root;

  beforeEach(async () => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    vi.mocked(fsList).mockResolvedValue(listing);
    vi.mocked(fsRead).mockResolvedValue({ content: "original", size: 8 });
    vi.mocked(fsWrite).mockResolvedValue(undefined);
    opener = document.createElement("button");
    document.body.appendChild(opener);
    opener.focus();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);

    await act(async () => {
      root.render(
        <I18nContext.Provider value={i18n}>
          <FileBrowser
            path="/work"
            wsId="workspace-1"
            tmId="terminal-1"
            onClose={() => undefined}
            notify={() => undefined}
            width={320}
            onWidthChange={() => undefined}
          />
        </I18nContext.Provider>,
      );
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    opener.remove();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("moves focus into the overlay and restores it after Escape closes", async () => {
    const onClose = vi.fn();
    await act(async () => {
      root.render(
        <I18nContext.Provider value={i18n}>
          <FileBrowser
            path="/work"
            wsId="workspace-1"
            tmId="terminal-1"
            onClose={onClose}
            notify={() => undefined}
            width={320}
            onWidthChange={() => undefined}
          />
        </I18nContext.Provider>,
      );
    });

    const closeButton = container.querySelector<HTMLButtonElement>(".th-files-head .th-btn-icon");
    if (!closeButton) throw new Error("File browser close button is missing");
    expect(document.activeElement).toBe(closeButton);

    act(() => {
      closeButton.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    });
    expect(onClose).toHaveBeenCalledOnce();

    act(() => root.render(<></>));
    expect(document.activeElement).toBe(opener);
  });

  it("guards header and Escape overlay closes while the editor is dirty", async () => {
    const onClose = vi.fn();
    vi.mocked(fsList).mockResolvedValue({
      path: "/files",
      parent: null,
      entries: [{ name: "note.txt", isDir: false, size: 8, modTime: "2026-01-01T00:00:00Z" }],
    });
    await act(async () => root.render(
      <I18nContext.Provider value={i18n}>
        <FileBrowser path="/files" wsId="workspace-1" tmId="terminal-1" onClose={onClose} notify={() => undefined} width={320} onWidthChange={() => undefined} />
      </I18nContext.Provider>,
    ));
    const row = container.querySelector<HTMLButtonElement>(".th-files-name--link");
    await act(async () => row?.click());
    expect(container.querySelector(".th-editor-area")).toBeNull();
    await act(async () => Array.from(container.querySelectorAll<HTMLButtonElement>(".th-viewer-toolbar button")).find(button => button.getAttribute("aria-label") === "Edit")?.click());
    const editor = container.querySelector<HTMLTextAreaElement>(".th-editor-area");
    if (!editor) throw new Error("File editor textarea is missing");
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(editor, "changed");
      editor.dispatchEvent(new Event("input", { bubbles: true }));
    });

    await act(async () => container.querySelector<HTMLButtonElement>(".th-files-head button")?.click());
    expect(onClose).not.toHaveBeenCalled();
    expect(document.querySelector('[role="dialog"]')).not.toBeNull();
    await act(async () => document.querySelector<HTMLButtonElement>('[role="dialog"] .th-btn--ghost')?.click());
    await act(async () => editor.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })));
    expect(onClose).not.toHaveBeenCalled();
    await act(async () => document.querySelector<HTMLButtonElement>('[role="dialog"] .th-btn--danger')?.click());
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("restores focus to the invoking file row after discarding an editor close", async () => {
    vi.mocked(fsList).mockResolvedValue({
      path: "/files",
      parent: null,
      entries: [{ name: "note.txt", isDir: false, size: 8, modTime: "2026-01-01T00:00:00Z" }],
    });
    await act(async () => root.render(
      <I18nContext.Provider value={i18n}>
        <FileBrowser path="/files" wsId="workspace-1" tmId="terminal-1" onClose={() => undefined} notify={() => undefined} width={320} onWidthChange={() => undefined} />
      </I18nContext.Provider>,
    ));
    const row = container.querySelector<HTMLButtonElement>(".th-files-name--link");
    await act(async () => row?.click());
    expect(container.querySelector(".th-editor-area")).toBeNull();
    await act(async () => Array.from(container.querySelectorAll<HTMLButtonElement>(".th-viewer-toolbar button")).find(button => button.getAttribute("aria-label") === "Edit")?.click());
    const editor = container.querySelector<HTMLTextAreaElement>(".th-editor-area");
    if (!editor) throw new Error("File editor textarea is missing");
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set?.call(editor, "changed");
      editor.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => container.querySelector<HTMLButtonElement>(".th-editor-head .th-btn-icon")?.click());
    await act(async () => document.querySelector<HTMLButtonElement>('[role="dialog"] .th-btn--danger')?.click());

    expect(container.querySelector(".th-viewer")).not.toBeNull();
    await act(async () => Array.from(container.querySelectorAll<HTMLButtonElement>(".th-viewer-toolbar button")).find(button => button.getAttribute("aria-label") === "Back / Close")?.click());
    expect(document.activeElement).toBe(container.querySelector(".th-files-name--link"));
  });
  it("does not replay a cancelled browser close when re-entering edit", async () => {
    const close = vi.fn();
    vi.mocked(fsList).mockResolvedValue({ ...listing, entries: [{ name: "note.txt", isDir: false, size: 8, modTime: "2026-01-01T00:00:00Z" }] });
    await act(async () => root.render(<I18nContext.Provider value={i18n}><FileBrowser path="/files" wsId="w" tmId="t" onClose={close} notify={vi.fn()} width={320} onWidthChange={vi.fn()} /></I18nContext.Provider>));
    await act(async () => container.querySelector<HTMLButtonElement>(".th-files-name--link")!.click());
    const edit = () => container.querySelector<HTMLButtonElement>('[aria-label="Edit"]')!.click();
    await act(async () => edit());
    const editor = container.querySelector<HTMLTextAreaElement>("textarea")!;
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(editor, "changed");
      editor.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => container.querySelector<HTMLButtonElement>(".th-files-head button")!.click());
    await act(async () => document.querySelector<HTMLButtonElement>('[role="dialog"] .th-btn--ghost')!.click());
    await act(async () => container.querySelector<HTMLButtonElement>(".th-editor-head .th-btn-icon")!.click());
    await act(async () => document.querySelector<HTMLButtonElement>('[role="dialog"] .th-btn--danger')!.click());
    await act(async () => edit());
    expect(close).not.toHaveBeenCalled();
    expect(container.querySelector("textarea")).not.toBeNull();
  });

  it.each([["sound.mp3", "audio"], ["movie.mp4", "video"], ["page.html", "iframe"]])("includes %s in the focus trap after close and supports reverse Tab", async (name, tag) => {
    vi.spyOn(HTMLElement.prototype, "getClientRects").mockReturnValue([{}] as unknown as DOMRectList);
    vi.mocked(fsList).mockResolvedValue({ ...listing, entries: [{ name, isDir: false, size: 8, modTime: "2026-01-01T00:00:00Z" }] });
    await act(async () => root.render(<I18nContext.Provider value={i18n}><FileBrowser path="/files" wsId="w" tmId="t" onClose={vi.fn()} notify={vi.fn()} width={320} onWidthChange={vi.fn()} /></I18nContext.Provider>));
    await act(async () => container.querySelector<HTMLButtonElement>(".th-files-name--link")!.click());
    const close = container.querySelector<HTMLButtonElement>(".th-viewer-toolbar button:last-child")!;
    const preview = container.querySelector<HTMLElement>(tag)!;
    for (const [attribute, value] of [["tabindex", "-1"], ["hidden", ""], ["disabled", ""]] as const) {
      const excluded = document.createElement("button");
      excluded.textContent = "excluded";
      excluded.setAttribute(attribute, value);
      container.querySelector(".th-viewer-body")!.appendChild(excluded);
    }
    // jsdom does not implement native Tab/media focus: emulate the browser's
    // default move only when the focus trap has not cancelled that key.
    const nativeFocus = preview.focus.bind(preview);
    vi.spyOn(preview, "focus").mockImplementation(() => { preview.tabIndex = 0; nativeFocus(); preview.removeAttribute("tabindex"); });
    const tab = (from: HTMLElement, shift = false) => {
      from.focus();
      const event = new KeyboardEvent("keydown", { key: "Tab", shiftKey: shift, bubbles: true, cancelable: true });
      act(() => { from.dispatchEvent(event); });
      return event;
    };
    expect(tab(close).defaultPrevented).toBe(false);
    preview.focus(); expect(document.activeElement).toBe(preview);
    expect(tab(preview, true).defaultPrevented).toBe(false);
    close.focus(); expect(document.activeElement).toBe(close);
    expect(tab(preview).defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(container.querySelector('[role="separator"]'));
    tab(container.querySelector<HTMLElement>('[role="separator"]')!, true);
    expect(document.activeElement).toBe(preview);
  });
});
