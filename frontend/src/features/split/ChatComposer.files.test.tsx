import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { I18nContext } from "../../i18n";
import { uploadFiles } from "../terminal/terminal";
import { ChatComposer } from "./ChatComposer";
import { i18n, chatSession } from "./chatPaneTestHarness";
vi.mock("../terminal/terminal", () => ({ uploadFiles: vi.fn() }));
let container: HTMLDivElement;
let root: ReturnType<typeof createRoot>;
const submit = vi.fn(() => true);
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.clearAllMocks();
  container = document.createElement("div"); document.body.append(container); root = createRoot(container);
  act(() => root.render(<I18nContext.Provider value={i18n}><ChatComposer session={chatSession} commands={[]} running={false} isCompacting={false} retryDraft={null} onSubmit={submit} onSteer={() => true} onStop={() => {}} provider="omo" cwd="/work" /></I18nContext.Provider>));
});
afterEach(() => { act(() => root.unmount()); container.remove(); vi.unstubAllGlobals(); });
function pick(file: File, kind: "picker" | "paste" | "drop") {
  if (kind === "picker") {
    const input = container.querySelector("input[type=file]")!;
    Object.defineProperty(input, "files", { configurable: true, value: [file] }); input.dispatchEvent(new Event("change", { bubbles: true }));
  } else {
    const event = new Event(kind, { bubbles: true, cancelable: true });
    Object.defineProperty(event, kind === "paste" ? "clipboardData" : "dataTransfer", { value: { files: [file] } });
    container.querySelector(kind === "paste" ? "textarea" : "form")!.dispatchEvent(event);
  }
}
for (const kind of ["picker", "paste", "drop"] as const) it(`uploads non-image via ${kind}, blocks send, then appends stable block`, async () => {
  let resolve!: (paths: readonly string[]) => void;
  vi.mocked(uploadFiles).mockImplementation(() => new Promise(done => { resolve = done; }));
  const file = new File(["pdf"], "report.pdf", { type: "application/pdf" });
  await act(async () => pick(file, kind));
  expect(uploadFiles).toHaveBeenCalledWith("workspace-1", "chat-1", [file], expect.objectContaining({ dir: "uploads", onProgress: expect.any(Function) }));
  expect(container.querySelector("progress")).not.toBeNull();
  expect(container.querySelector<HTMLButtonElement>(".th-chat-send-btn")!.disabled).toBe(true);
  act(() => container.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));
  expect(submit).not.toHaveBeenCalled();
  await act(async () => resolve(["uploads/report (1).pdf"]));
  expect(container.textContent).toContain("report.pdf");
  act(() => container.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));
  expect(submit).toHaveBeenCalledWith({ text: "\n\n[첨부 파일]\n- uploads/report (1).pdf", image: null });
  expect(container.querySelector(".th-chat-file-chip")).toBeNull();
});
it("retains failed chip, retries and removes without deleting", async () => {
  vi.mocked(uploadFiles).mockRejectedValueOnce(new Error("server full")).mockResolvedValueOnce(["uploads/a.docx"]);
  await act(async () => pick(new File(["doc"], "a.docx"), "picker"));
  expect(container.querySelector("[role=alert]")?.textContent).toBe("server full");
  await act(async () => container.querySelector<HTMLButtonElement>('.th-chat-file-chip .th-btn--ghost')!.click());
  expect(uploadFiles).toHaveBeenCalledTimes(2);
  act(() => container.querySelector<HTMLButtonElement>('.th-chat-file-chip .th-btn-icon')!.click());
  expect(container.querySelector(".th-chat-file-chip")).toBeNull();
});
