import { act } from "react";
import { createRoot } from "react-dom/client";
import type { Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SessionTree } from "./SessionTree";
import type { SessionTreeProps } from "./SessionTree";
import type { Workspace, WorkspaceSession } from "../features/workspace/workspace";

describe("SessionTree indexed row rendering", () => {
  let container: HTMLDivElement;
  let root: Root;
  beforeEach(() => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });
  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.unstubAllGlobals();
  });

  function props(workspace: Workspace, rows: readonly WorkspaceSession[]): SessionTreeProps {
    return {
      workspaces: [workspace], activeTerminalId: "chat-0", placedSessions: new Set(),
      liveSessions: new Set(), expanded: new Set([workspace.id]),
      sessionLists: new Map([[workspace.id, rows]]), sessionPages: new Map(),
      onToggle: vi.fn(), onLoadMoreSessions: vi.fn(), onSelect: vi.fn(),
      onAddTerminal: vi.fn(), onDeleteWorkspace: vi.fn(), onDeleteTerminal: vi.fn(),
      onRenameWorkspace: async () => undefined, onRenameTerminal: async () => undefined,
      notify: vi.fn(),
    };
  }

  it("uses linear id reads for 1,000 rows, reuses the index and skips collapsed row work", () => {
    let idReads = 0;
    let rowReads = 0;
    const chats = Array.from({ length: 1000 }, (_, index) => ({
      get id() { idReads++; return `chat-${index}`; },
      name: `Stored ${index}`, provider: "omo" as const,
    }));
    const rows: WorkspaceSession[] = chats.map((chat) => ({
      id: chat.id, get name() { rowReads++; return chat.name; }, source: "stored", recencyMs: 0,
    }));
    const initial = props({ id: "ws", name: "Workspace", path: "/work", chats }, rows);
    idReads = 0;
    act(() => root.render(<SessionTree {...initial} />));
    expect(container.querySelectorAll("[data-session-id]")).toHaveLength(1000);
    expect(idReads).toBeLessThanOrEqual(2000);
    expect(rowReads).toBeGreaterThanOrEqual(1000);
    act(() => container.querySelector<HTMLButtonElement>('[data-session-id="chat-999"] .th-tree-activation')?.click());
    expect(initial.onSelect).toHaveBeenCalledWith(initial.workspaces[0], chats[999]);

    idReads = 0;
    rowReads = 0;
    act(() => root.render(<SessionTree {...initial} expanded={new Set()} />));
    expect(container.querySelectorAll("[data-session-id]")).toHaveLength(0);
    expect(rowReads).toBe(0);
    // The aggregate still reads each chat once, but the memoized index is reused.
    expect(idReads).toBe(1000);

    idReads = 0;
    act(() => root.render(<SessionTree {...initial} />));
    expect(container.querySelectorAll("[data-session-id]")).toHaveLength(1000);
    expect(idReads).toBe(1000);
    const replacement = { ...chats[0]!, name: "Updated" };
    act(() => root.render(<SessionTree {...initial} workspaces={[{ ...initial.workspaces[0]!, chats: [replacement] }]} />));
    act(() => container.querySelector<HTMLButtonElement>('[data-session-id="chat-0"] .th-tree-activation')?.click());
    expect(initial.onSelect).toHaveBeenLastCalledWith(expect.anything(), replacement);
  });

  it("keeps desktop row actions after primary activation in the natural tab order", () => {
    const initial = props({ id: "ws", name: "Workspace", path: "/work", chats: [
      { id: "chat-0", name: "Stored", provider: "omo" },
    ] }, [{ id: "chat-0", name: "Stored", source: "stored", recencyMs: 0 }]);
    act(() => root.render(<SessionTree {...initial} />));
    const row = container.querySelector('[data-session-id="chat-0"]')!;
    const buttons = [...row.querySelectorAll<HTMLButtonElement>("button")];
    expect(buttons).toHaveLength(3);
    expect(buttons[0]?.classList.contains("th-tree-activation")).toBe(true);
    buttons[0]!.focus();
    expect(row.matches(":focus-within")).toBe(true);
    for (const button of buttons) {
      expect(button.disabled).toBe(false);
      expect(button.tabIndex).toBe(0);
      expect(button.closest('[inert], [hidden], [aria-hidden="true"]')).toBeNull();
      button.focus();
      expect(document.activeElement).toBe(button);
    }
    act(() => buttons[2]!.click());
    expect(initial.onDeleteTerminal).toHaveBeenCalledWith(initial.workspaces[0], initial.workspaces[0]!.chats[0]);
  });
});
