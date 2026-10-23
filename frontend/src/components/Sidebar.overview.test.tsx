import { act } from "react";
import { createRoot } from "react-dom/client";
import type { Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Sidebar } from "./Sidebar";
import { useMediaQuery } from "../lib/useMediaQuery";
import type { Terminal, Workspace, WorkspaceSession } from "../features/workspace/workspace";

vi.mock("../lib/useMediaQuery", () => ({ useMediaQuery: vi.fn() }));

const workspace: Workspace = {
  id: "ws-1",
  name: "Workspace",
  path: "/work",
  chats: [{ id: "tm-1", name: "Stored session", provider: "omo" }],
};

const LIVE_RESPONSE = {
  sessions: [
    {
      id: "tm-1",
      title: "Refactor auth",
      running: { agents: 1 }, done: 0, last_line: "ls",
    },
  ],
};

/** A live (process-attached) session whose only task is done: zero running agents. */
const IDLE_LIVE_RESPONSE = {
  sessions: [
    {
      id: "disk-1",
      title: "Idle attached session",
      active: false,
      running: { agents: 0 }, done: 1,
    },
  ],
};

const catalogRow: WorkspaceSession = {
  id: "disk-1",
  name: "Disk session",
  source: "stored",
  recencyMs: 1,
};

const secondCatalogRow: WorkspaceSession = {
  id: "disk-2",
  name: "Second disk session",
  source: "stored",
  recencyMs: 2,
};

const thirdCatalogRow: WorkspaceSession = {
  id: "disk-3",
  name: "Third disk session",
  source: "stored",
  recencyMs: 3,
};

/** One working session plus two idle live sessions in the same poll. The
 * idle rows mirror the harness fixture: attached (active: false) with no
 * task and no DAG at all. */
const MIXED_LIVE_RESPONSE = {
  sessions: [
    liveEntry("disk-1", "Working session", "running"),
    liveEntry("disk-2", "Idle older", null, 1000, false),
    liveEntry("disk-3", "Idle recent", null, 500, false),
  ],
};

/** An unattached auto-enrolled daemon session arrives on the live feed keyed
 * by its stored chat id with the main agent working and two child agents. */
const DAEMON_LIVE_RESPONSE = {
  sessions: [
    { id: "chat-daemon", title: "Daemon-enrolled chat", active: true, running: { agents: 2 }, done: 0 },
  ],
};

function liveEntry(id: string, title: string, status: "running" | "completed" | null, updatedAgoMs = 1000, active?: boolean): unknown {
  return {
    id,
    title,
    // The server flags attached sessions with an explicit active boolean;
    // rows without it are finished history, not live sessions.
    ...(active === undefined ? {} : { active }),
    running: { agents: status === "running" ? 1 : 0 },
    done: status === "completed" ? 1 : 0,
    last_activity_ms: 2000 - updatedAgoMs,
  };
}

function okResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    text: async () => JSON.stringify(body),
  } as unknown as Response;
}

describe("Sidebar pinned running sessions", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    vi.mocked(useMediaQuery).mockReturnValue(false);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  interface RenderOptions {
    readonly chats?: readonly Terminal[];
    readonly activeId?: string;
    readonly sessions?: readonly WorkspaceSession[];
    readonly onOpenSession?: (ws: Workspace, session: WorkspaceSession) => Promise<"opened" | "session-active" | void>;
  }

  function renderSidebar(
    onSelect: (ws: Workspace, tm: Terminal) => void,
    options: RenderOptions = {},
  ): void {
    const ws: Workspace = { ...workspace, chats: options.chats ?? workspace.chats };
    const sessions: readonly WorkspaceSession[] = options.sessions ?? [
      { id: "tm-1", name: "Stored session", source: "stored", recencyMs: 1 },
    ];
    act(() => {
      root.render(
        <Sidebar
          collapsed={false}
          onToggleCollapse={() => undefined}
          workspaces={[ws]}
          activeTerminalId={options.activeId ?? null}
          placedSessions={new Set(["tm-1", "disk-1"])}
          liveSessions={new Set(["tm-1"])}
          expanded={new Set(["ws-1"])}
          sessionLists={new Map([["ws-1", sessions]])}
          sessionPages={new Map()}
          onToggleExpanded={() => undefined}
          onLoadMoreSessions={() => undefined}
          onSelectTerminal={onSelect}
          onOpenSession={options.onOpenSession ?? (async () => undefined)}
          onAddWorkspace={() => undefined}
          onAddTerminal={() => undefined}
          onDeleteWorkspace={() => undefined}
          onDeleteTerminal={() => undefined}
          onRenameWorkspace={async () => undefined}
          onRenameTerminal={async () => undefined}
          onLogout={() => undefined}
          notify={() => undefined}
        />,
      );
    });
  }

  it("renders each live conversation once in the tree and preserves stored-chat activation", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => okResponse(LIVE_RESPONSE)));
    const onSelect = vi.fn();
    renderSidebar(onSelect, { activeId: "tm-1" });
    await act(async () => {});
    expect(container.querySelector(".th-sidebar-live")).toBeNull();
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(container.querySelectorAll('[data-session-id="tm-1"]')).toHaveLength(1);
    expect(container.querySelectorAll(".th-tree-node--active")).toHaveLength(1);
    expect(container.querySelectorAll(".th-tree-indicator")).toHaveLength(1);
    expect(container.querySelector('.th-tree-placed[aria-label="sidebar.tm.openInPane"]')).not.toBeNull();
    const row = container.querySelector('[data-session-id="tm-1"]');
    expect(row?.querySelector(".th-tree-running")?.textContent).toBe("1");
    expect(container.querySelector(".th-tree-running--workspace")?.textContent).toBe("1");
    act(() => row?.querySelector<HTMLButtonElement>(".th-tree-activation")?.click());
    expect(onSelect).toHaveBeenCalledExactlyOnceWith(workspace, workspace.chats[0]);
    expect([...container.querySelectorAll<HTMLButtonElement>(".th-sidebar-nav-actions button")].map(button => button.title))
      .toEqual(["sidebar.addWorkspace", "sidebar.collapse"]);
  });

  it("keeps working-first live order and idle connection without a running chip", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => okResponse(MIXED_LIVE_RESPONSE)));
    renderSidebar(() => undefined, { chats: [], sessions: [catalogRow, secondCatalogRow, thirdCatalogRow] });
    await act(async () => {});
    const rows = [...container.querySelectorAll<HTMLElement>("[data-session-id]")];
    expect(rows.map(row => row.dataset["sessionId"])).toEqual(["disk-1", "disk-3", "disk-2"]);
    expect(rows[0]?.querySelector(".th-tree-running")).not.toBeNull();
    expect(rows[1]?.querySelector(".th-tree-running")).toBeNull();
    expect(rows[1]?.querySelector(".th-tree-live")).not.toBeNull();
    expect(rows[2]?.querySelector(".th-tree-running")).toBeNull();
  });

  it("excludes unresolvable engine identities as before", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => okResponse({ sessions: [{ id: "uuid", title: "Unopenable", running: { agents: 2 } }] })));
    renderSidebar(() => undefined, { chats: [], sessions: [catalogRow] });
    await act(async () => {});
    expect(container.querySelector('[data-session-id="uuid"]')).toBeNull();
    expect(container.querySelector(".th-tree-running--workspace")).toBeNull();
  });

  it("excludes unresolvable engine identities from a mixed live feed and its counts", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => okResponse({ sessions: [
      liveEntry("disk-1", "Openable", "running"),
      { id: "uuid", title: "Unopenable", active: true, running: { agents: 2 } },
    ] })));
    renderSidebar(() => undefined, { chats: [], sessions: [catalogRow] });
    await act(async () => {});
    expect(container.querySelector('[data-session-id="uuid"]')).toBeNull();
    expect(container.querySelectorAll('[data-session-id]')).toHaveLength(1);
    const row = container.querySelector('[data-session-id="disk-1"]');
    expect(row?.querySelector(".th-tree-live")).not.toBeNull();
    expect(row?.querySelector(".th-tree-running")?.textContent).toBe("1");
    expect(container.querySelector(".th-tree-running--workspace")?.textContent).toBe("1");
    expect(container.querySelector(".th-tree-count")?.textContent).toBe("1");
  });

  it("opens catalog-only conversations, exposes failure and retries without a second row", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => okResponse(IDLE_LIVE_RESPONSE)));
    const onOpenSession = vi.fn().mockRejectedValueOnce(new Error("load failed")).mockResolvedValue("opened");
    renderSidebar(() => undefined, { chats: [], sessions: [catalogRow], onOpenSession });
    await act(async () => {});
    const row = container.querySelector('[data-session-id="disk-1"]');
    await act(async () => row?.querySelector<HTMLButtonElement>(".th-tree-activation")?.click());
    expect(onOpenSession).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ id: "ws-1" }), catalogRow);
    expect(row?.querySelector('[role="status"]')?.textContent).toContain("sidebar.tm.openFailed");
    await act(async () => row?.querySelector<HTMLButtonElement>(".th-tree-retry")?.click());
    expect(onOpenSession).toHaveBeenCalledTimes(2);
    expect(row?.querySelector('[role="status"]')).toBeNull();
    expect(container.querySelectorAll('[data-session-id="disk-1"]')).toHaveLength(1);
  });

  it("keeps a catalog open pending on its one row and blocks duplicate activation", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => okResponse(IDLE_LIVE_RESPONSE)));
    let resolveOpen: (() => void) | undefined;
    const pending = new Promise<void>(resolve => { resolveOpen = resolve; });
    const onOpenSession = vi.fn(() => pending);
    renderSidebar(() => undefined, { chats: [], sessions: [catalogRow], onOpenSession });
    await act(async () => {});
    const button = container.querySelector<HTMLButtonElement>('[data-session-id="disk-1"] .th-tree-activation')!;
    act(() => { button.click(); button.click(); });
    expect(onOpenSession).toHaveBeenCalledTimes(1);
    expect(button.disabled).toBe(true);
    expect(button.getAttribute("aria-busy")).toBe("true");
    expect(container.querySelector('[role="status"]')?.textContent).toBe("sidebar.tm.opening");
    await act(async () => resolveOpen?.());
    expect(button.disabled).toBe(false);
    expect(container.querySelector('[role="status"]')).toBeNull();
  });

  it("includes live stored chats absent from the paged catalog exactly once", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => okResponse(DAEMON_LIVE_RESPONSE)));
    renderSidebar(() => undefined, { chats: [{ id: "chat-daemon", name: "Daemon-enrolled chat", provider: "omo" }], sessions: [] });
    await act(async () => {});
    expect(container.querySelectorAll('[data-session-id="chat-daemon"]')).toHaveLength(1);
    expect(container.querySelector('[data-session-id="chat-daemon"] .th-tree-running')?.textContent).toBe("2");
    expect(container.querySelector(".th-tree-running--workspace")?.textContent).toBe("2");
  });
});
