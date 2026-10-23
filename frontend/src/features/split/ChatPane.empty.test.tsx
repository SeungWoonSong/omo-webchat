import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { renderChatPane, setTextareaValue } from "./chatPaneTestHarness";

let cleanup = () => {};
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("inserts each empty-session suggestion, focuses the composer, never sends, and hides on the first message", () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("div"); document.body.append(host);
  const root = createRoot(host);
  cleanup = () => { act(() => root.unmount()); host.remove(); };
  const { deliver, sent } = renderChatPane(root);
  act(() => deliver({ type: "entries", sessionId: "chat-1", entries: [], final: true }));
  const buttons = Array.from(host.querySelectorAll<HTMLButtonElement>(".th-chat-suggestions button"));
  expect(buttons).toHaveLength(4);
  const editor = host.querySelector<HTMLTextAreaElement>("textarea")!;
  const before = [...sent];
  for (const button of buttons) {
    act(() => button.click());
    expect(editor.value).toBe(button.textContent);
    expect(document.activeElement).toBe(editor);
    expect(sent).toEqual(before);
    act(() => setTextareaValue(editor, ""));
  }
  act(() => buttons[0]!.click());
  expect(editor.value).toBe(buttons[0]!.textContent);
  act(() => deliver({ type: "message", sessionId: "chat-1", message: { role: "user", blocks: [{ kind: "text", text: "- first\n- second\n\n1. ordered" }] } }));
  expect(host.querySelector(".th-chat-session-empty")).toBeNull();
  expect(host.querySelector(".th-chat-msg--user .th-chat-markdown ul")).not.toBeNull();
  expect(host.querySelector(".th-chat-msg--user .th-chat-markdown ol")).not.toBeNull();
  act(() => deliver({ type: "message", sessionId: "chat-1", message: { role: "user", customType: "steer", blocks: [{ kind: "text", text: "More guidance" }] } }));
  const steer = host.querySelector('.th-chat-msg--steer[role="note"]');
  expect(steer?.firstElementChild?.className).toBe("th-chat-steer-mark");
  expect(steer?.firstElementChild?.textContent).toBe("chat.steerLabel");
  expect(steer?.querySelector(".th-chat-steer-text")?.textContent).toBe("More guidance");
});

for (const boundary of ["run.started", "streaming", "optimistic"] as const) it(`hides empty suggestions at the ${boundary} boundary before message content`, () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("div"); document.body.append(host);
  const root = createRoot(host);
  cleanup = () => { act(() => root.unmount()); host.remove(); };
  const { deliver } = renderChatPane(root);
  act(() => deliver({ type: "entries", sessionId: "chat-1", entries: [], final: true }));
  expect(host.querySelectorAll(".th-chat-suggestions button")).toHaveLength(4);
  if (boundary === "run.started") act(() => deliver({ type: "run.started", sessionId: "chat-1" }));
  else if (boundary === "streaming") act(() => deliver({ type: "state", sessionId: "chat-1", isStreaming: true, isCompacting: false }));
  else act(() => {
    setTextareaValue(host.querySelector("textarea")!, "First request");
  });
  if (boundary === "optimistic") act(() => host.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));
  expect(host.querySelector(".th-chat-session-empty")).toBeNull();
});
