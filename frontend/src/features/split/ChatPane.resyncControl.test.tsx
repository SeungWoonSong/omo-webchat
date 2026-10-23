import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nContext, translate, type I18nValue } from "../../i18n";
import type { ChatClientFrame, ChatConnector, ChatServerFrame } from "../../lib/chatWs";
import { ChatPane } from "./ChatPane";
import { useChatSession } from "./useChatSession";
import {
  chatSession,
  ControlledResizeObserver,
  requireElement,
} from "./chatPaneTestHarness";

const koI18n: I18nValue = {
  lang: "ko",
  setLang: () => undefined,
  font: "system",
  setFont: () => undefined,
  fontSize: 13,
  setFontSize: () => undefined,
  t: (key) => translate("ko", key),
};

type RenderOptions = {
  readonly failClose?: boolean;
};

describe("ChatPane resync control", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    ControlledResizeObserver.instances = [];
    vi.stubGlobal("ResizeObserver", ControlledResizeObserver);
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>(() => undefined)));
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
    ControlledResizeObserver.instances = [];
    container.remove();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const renderKoreanPane = (
    options: RenderOptions = {},
  ): {
    deliver: (frame: ChatServerFrame) => void;
    disconnect: () => void;
    sent: ChatClientFrame[];
  } => {
    let deliverFrame: ((frame: ChatServerFrame) => void) | undefined;
    let disconnectClient = (): void => undefined;
    const sent: ChatClientFrame[] = [];
    const connect: ChatConnector = (handlers) => {
      deliverFrame = handlers.onFrame;
      disconnectClient = () => handlers.onClose?.(1006);
      handlers.onOpen?.();
      return {
        send: (frame) => {
          sent.push(frame);
          return !(options.failClose && frame.type === "chat.close");
        },
        close: vi.fn(),
      };
    };

    act(() => {
      root.render(
        <I18nContext.Provider value={koI18n}>
          <ChatPane
            chatSession={chatSession}
            focused
            splitEnabled
            onFocus={() => undefined}
            onSplit={() => undefined}
            onClose={() => undefined}
            onOpenSidebar={() => undefined}
            connect={connect}
            notify={() => undefined}
          />
        </I18nContext.Provider>,
      );
    });

    return {
      deliver: (frame) => deliverFrame?.(frame),
      disconnect: () => disconnectClient(),
      sent,
    };
  };

  const renderKoreanSessionProbe = (
    options: RenderOptions = {},
  ): { deliver: (frame: ChatServerFrame) => void; resync: () => boolean } => {
    let deliverFrame: ((frame: ChatServerFrame) => void) | undefined;
    let resyncSession = (): boolean => false;
    const connect: ChatConnector = (handlers) => {
      deliverFrame = handlers.onFrame;
      handlers.onOpen?.();
      return {
        send: (frame) => !(options.failClose && frame.type === "chat.close"),
        close: vi.fn(),
      };
    };

    function SessionProbe() {
      const chat = useChatSession(chatSession, connect);
      resyncSession = chat.resync;
      return chat.error ? <div className="th-chat-error">{chat.error}</div> : null;
    }

    act(() => {
      root.render(
        <I18nContext.Provider value={koI18n}>
          <SessionProbe />
        </I18nContext.Provider>,
      );
    });

    return {
      deliver: (frame) => deliverFrame?.(frame),
      resync: () => resyncSession(),
    };
  };

  const resyncButton = (): HTMLButtonElement => {
    const existing = container.querySelector<HTMLButtonElement>(".th-chat-resync-btn");
    if (existing) return existing;
    const more = requireElement(
      container.querySelector<HTMLButtonElement>(".th-termhead-more-btn"),
      "header overflow trigger",
    );
    act(() => more.click());
    return requireElement(
      container.querySelector<HTMLButtonElement>(".th-chat-resync-btn"),
      "resync control",
    );
  };

  it("moves secondary header actions into the overflow menu with localized accessible names", () => {
    container.style.width = "320px";
    renderKoreanPane();

    expect(container.querySelector(".th-files-toggle")).not.toBeNull();
    expect(container.querySelector(".th-termhead-close-btn")).not.toBeNull();
    expect(container.querySelector(".th-chat-resync-btn")).toBeNull();
    expect(container.querySelector(".th-disconnect-btn")).toBeNull();
    expect(container.querySelector(".th-termhead-split-btn")).toBeNull();

    const more = requireElement(
      container.querySelector<HTMLButtonElement>(".th-termhead-more-btn"),
      "header overflow trigger",
    );
    expect(more.getAttribute("aria-label")).toBe(translate("ko", "chat.header.moreActions"));
    expect(more.getAttribute("aria-expanded")).toBe("false");

    const button = resyncButton();
    expect(more.getAttribute("aria-expanded")).toBe("true");
    const accessibleName = translate("ko", "chat.header.resync");
    expect(button.classList.contains("th-tree-overflow-item")).toBe(true);
    expect(button.getAttribute("aria-label")).toBe(accessibleName);
    expect(button.title).toBe(accessibleName);
    expect(button.querySelector(".th-chat-resync-icon")?.getAttribute("aria-hidden")).toBe("true");
    expect(container.querySelectorAll(".th-termhead-split-btn")).toHaveLength(2);
    const disconnectBtn = requireElement(
      container.querySelector<HTMLButtonElement>(".th-disconnect-btn"),
      "disconnect control",
    );
    expect(disconnectBtn.getAttribute("aria-label")).toBe(
      translate("ko", "chat.header.disconnect"),
    );

    act(() => {
      disconnectBtn.focus();
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    });
    expect(container.querySelector(".th-termhead-overflow")).toBeNull();
    expect(document.activeElement).toBe(more);
  });

  it("consumes Escape from a focused overflow action and restores its trigger", () => {
    renderKoreanPane();
    resyncButton();
    const button = requireElement(
      container.querySelector<HTMLButtonElement>(".th-disconnect-btn"),
      "enabled overflow action",
    );
    const more = requireElement(
      container.querySelector<HTMLButtonElement>(".th-termhead-more-btn"),
      "header overflow trigger",
    );
    const escape = new KeyboardEvent("keydown", {
      key: "Escape", bubbles: true, cancelable: true,
    });
    act(() => {
      button.focus();
      button.dispatchEvent(escape);
    });
    expect(escape.defaultPrevented).toBe(true);
    expect(container.querySelector(".th-termhead-overflow")).toBeNull();
    expect(more.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(more);
  });

  it("bounds the unsupported-popover fallback in a 160px pane and keeps all actions focusable", () => {
    expect(typeof HTMLElement.prototype.showPopover).not.toBe("function");
    vi.stubGlobal("innerWidth", 320);
    vi.stubGlobal("innerHeight", 160);
    let triggerTop = 8;
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      const trigger = this.classList.contains("th-termhead-more-btn");
      const anchor = this.classList.contains("th-termhead-menu");
      return new DOMRect(trigger || anchor ? 280 : 0, trigger || anchor ? triggerTop : 0,
        trigger || anchor ? 32 : 320, trigger || anchor ? 32 : 160);
    });
    vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(192);
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(192);
    vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(200);
    const { deliver } = renderKoreanPane();
    act(() => deliver({ type: "entries", sessionId: chatSession.id, entries: [], final: true }));
    resyncButton();
    const menu = requireElement(container.querySelector<HTMLDivElement>(".th-termhead-overflow"), "menu");
    expect(menu.hasAttribute("popover")).toBe(false);
    expect(menu.style.position).toBe("absolute");
    expect(menu.style.top).toBe("32px");
    expect(menu.style.left).toBe("-168px");
    expect(menu.style.maxHeight).toBe("120px");
    expect(menu.style.maxWidth).toBe("320px");
    const actions = [...menu.querySelectorAll("button")];
    expect(actions).toHaveLength(4);
    expect(actions.map(button => button.getAttribute("aria-label"))).toEqual([
      "chat.header.resync", "chat.header.splitRight", "chat.header.splitDown", "chat.header.disconnect",
    ].map(key => translate("ko", key)));
    for (const button of actions) {
      expect(button.disabled).toBe(false);
      expect(button.tabIndex).toBe(0);
      button.focus();
      expect(document.activeElement).toBe(button);
    }
    // A moved pane/trigger flips upward on captured scroll without remounting.
    triggerTop = 120;
    act(() => container.dispatchEvent(new Event("scroll")));
    expect(menu.style.top).toBe("-120px");
    expect(menu.style.maxHeight).toBe("120px");
    triggerTop = 8;
    vi.stubGlobal("innerHeight", 90);
    act(() => window.dispatchEvent(new Event("resize")));
    expect(menu.style.maxHeight).toBe("50px");
    expect(menu.querySelectorAll("button")).toHaveLength(4);
    act(() => actions[3]!.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })));
    expect(container.querySelector(".th-termhead-overflow")).toBeNull();
    expect(document.activeElement).toBe(container.querySelector(".th-termhead-more-btn"));
  });

  it("presents only a manual resync as busy", () => {
    const { deliver } = renderKoreanPane();
    act(() => {
      deliver({ type: "entries", sessionId: chatSession.id, entries: [], final: true });
    });

    const button = resyncButton();
    expect(button.disabled).toBe(false);

    act(() => button.click());

    const busyButton = resyncButton();
    expect(busyButton.disabled).toBe(true);
    expect(busyButton.getAttribute("aria-busy")).toBe("true");
    expect(busyButton.getAttribute("aria-label")).toBe(translate("ko", "chat.header.resyncBusy"));
    expect(busyButton.title).toBe(translate("ko", "chat.header.resyncBusy"));
  });

  it("stays disabled but not busy during attach and after disconnect", () => {
    const { deliver, disconnect } = renderKoreanPane();
    const button = resyncButton();
    const idleLabel = translate("ko", "chat.header.resync");

    expect(button.disabled).toBe(true);
    expect(button.getAttribute("aria-busy")).toBe("false");
    expect(button.getAttribute("aria-label")).toBe(idleLabel);

    act(() => {
      deliver({ type: "entries", sessionId: chatSession.id, entries: [], final: true });
    });
    expect(button.disabled).toBe(false);

    act(() => disconnect());

    expect(button.disabled).toBe(true);
    expect(button.getAttribute("aria-busy")).toBe("false");
    expect(button.getAttribute("aria-label")).toBe(idleLabel);
  });

  it.each([
    {
      name: "an active assistant response",
      key: "chat.resyncBusyResponding",
      prepare: (deliver: (frame: ChatServerFrame) => void) =>
        deliver({ type: "run.started", sessionId: chatSession.id }),
      failClose: false,
    },
    {
      name: "active conversation compaction",
      key: "chat.resyncBusyCompacting",
      prepare: (deliver: (frame: ChatServerFrame) => void) =>
        deliver({ type: "compaction.started", sessionId: chatSession.id }),
      failClose: false,
    },
    {
      name: "a transport send failure",
      key: "chat.resyncError",
      prepare: () => undefined,
      failClose: true,
    },
  ])("surfaces translated text for $name", ({ key, prepare, failClose }) => {
    const { deliver, resync } = renderKoreanSessionProbe({ failClose });
    act(() => {
      deliver({ type: "entries", sessionId: chatSession.id, entries: [], final: true });
    });
    act(() => prepare(deliver));
    act(() => resync());

    const expected = translate("ko", key);
    const error = requireElement(
      container.querySelector(".th-chat-error"),
      "localized resync error",
    );
    expect(error.textContent).toBe(expected);
  });
});
