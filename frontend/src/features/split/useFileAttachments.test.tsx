import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { I18nContext } from "../../i18n";
import { uploadFiles } from "../terminal/terminal";
import { ChatComposer } from "./ChatComposer";
import { SessionDraftProvider, useSessionDraft } from "./sessionDraft";
import { useFileAttachments } from "./useFileAttachments";
import { chatSession, i18n } from "./chatPaneTestHarness";

vi.mock("../terminal/terminal", () => ({ uploadFiles: vi.fn() }));
const a = { ...chatSession, id: "a" };
const b = { ...chatSession, id: "b" };
const sessions = new Map([[a.id, a], [b.id, b]]);
let container: HTMLDivElement;
let root: ReturnType<typeof createRoot>;
let files: ReturnType<typeof useFileAttachments>;
let draft: ReturnType<typeof useSessionDraft>;
function Probe({ session }: { readonly session: typeof a }) {
  files = useFileAttachments(session);
  draft = useSessionDraft(session);
  return null;
}
function render(session: typeof a, mounted = true) {
  root.render(<I18nContext.Provider value={i18n}><SessionDraftProvider sessions={sessions}>{mounted && <Probe session={session} />}</SessionDraftProvider></I18nContext.Provider>);
}
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  container = document.createElement("div"); document.body.append(container); root = createRoot(container);
  vi.mocked(uploadFiles).mockResolvedValue(["uploads/proof.pdf"]);
});
afterEach(() => { act(() => root.unmount()); container.remove(); vi.unstubAllGlobals(); vi.resetAllMocks(); });
it("preserves pending attachment drafts through A to B to A and composer remount, clears on send", async () => {
  await act(async () => render(a));
  await act(async () => { draft.setInput("A draft"); files.add([new File(["proof"], "proof.pdf")]); });
  const id = files.items[0]!.id;
  await act(async () => render(b));
  expect(files.items).toEqual([]);
  expect(draft.input).toBe("");
  await act(async () => render(a));
  expect(draft.input).toBe("A draft");
  expect(files.paths).toEqual(["uploads/proof.pdf"]);
  expect(files.items[0]!.id).toBe(id);
  await act(async () => render(a, false));
  await act(async () => render(a));
  expect(files.paths).toEqual(["uploads/proof.pdf"]);
  const submit = vi.fn((_draft: import("./chatSessionTypes").ChatDraft) => true);
  await act(async () => root.render(<I18nContext.Provider value={i18n}><SessionDraftProvider sessions={sessions}><ChatComposer session={a} commands={[]} running={false} isCompacting={false} retryDraft={null} onSubmit={submit} onSteer={() => false} onStop={() => {}} provider="omo" cwd="/work" /></SessionDraftProvider></I18nContext.Provider>));
  await act(async () => container.querySelector<HTMLButtonElement>(".th-chat-send-btn")!.click());
  expect(submit.mock.calls[0]).toEqual([{ text: "A draft\n\n[첨부 파일]\n- uploads/proof.pdf", image: null }]);
  await act(async () => render(a));
  expect(files.items).toEqual([]);
  expect(draft.input).toBe("");
});
it("keeps interrupted uploads retryable and ignores late completions from another session", async () => {
  let resolve!: (paths: readonly string[]) => void;
  vi.mocked(uploadFiles).mockReturnValueOnce(new Promise(done => { resolve = done; }));
  await act(async () => render(a));
  await act(async () => files.add([new File(["proof"], "proof.pdf")]));
  const signal = vi.mocked(uploadFiles).mock.calls[0]![3]!.signal!;
  await act(async () => render(b));
  expect(signal.aborted).toBe(true);
  await act(async () => resolve(["uploads/late.pdf"]));
  expect(files.items).toEqual([]);
  await act(async () => render(a));
  expect(files.items[0]!.error).toBe("chat.uploadNetworkError");
  expect(files.blocked).toBe(true);
  await act(async () => files.retry(files.items[0]!.id));
  expect(files.paths).toEqual(["uploads/proof.pdf"]);
});
