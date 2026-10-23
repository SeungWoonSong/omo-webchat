import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { modalStack } from "../../components/modalStack";
import { ControlledResizeObserver, renderChatPane, requireElement } from "./chatPaneTestHarness";

describe("ChatPane header overflow modal dismissal", () => {
  let container: HTMLDivElement;
  let root: Root;
  const showPopover = vi.fn();
  const hidePopover = vi.fn();
  const originalShow = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "showPopover");
  const originalHide = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "hidePopover");

  beforeEach(() => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    vi.stubGlobal("ResizeObserver", ControlledResizeObserver);
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>(() => undefined)));
    Object.defineProperty(HTMLElement.prototype, "showPopover", { configurable: true, value: showPopover });
    Object.defineProperty(HTMLElement.prototype, "hidePopover", { configurable: true, value: hidePopover });
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    ControlledResizeObserver.instances = [];
    if (originalShow) Object.defineProperty(HTMLElement.prototype, "showPopover", originalShow);
    else Reflect.deleteProperty(HTMLElement.prototype, "showPopover");
    if (originalHide) Object.defineProperty(HTMLElement.prototype, "hidePopover", originalHide);
    else Reflect.deleteProperty(HTMLElement.prototype, "hidePopover");
    vi.restoreAllMocks();
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    expect(modalStack.size()).toBe(0);
  });

  function openMenu() {
    const trigger = requireElement(container.querySelector<HTMLButtonElement>(".th-termhead-more-btn"), "menu trigger");
    act(() => { trigger.focus(); trigger.click(); });
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(showPopover).toHaveBeenCalled();
    const action = requireElement(container.querySelector<HTMLButtonElement>(".th-termhead-overflow button"), "menu action");
    action.focus();
    return trigger;
  }

  it("hides the top-layer menu on server approval arrival without taking dialog focus", () => {
    const { deliver } = renderChatPane(root);
    const trigger = openMenu();
    const triggerFocus = vi.spyOn(trigger, "focus");
    act(() => deliver({ type: "approval", sessionId: "chat-1", id: "approval-menu", method: "select",
      title: "Allow bash?", options: ["Allow", "Block"] }));
    const dialog = requireElement(document.querySelector<HTMLElement>('[role="dialog"][aria-modal="true"]'), "approval dialog");
    expect(hidePopover).toHaveBeenCalled();
    expect(container.querySelector(".th-termhead-overflow")).toBeNull();
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(triggerFocus).not.toHaveBeenCalled();
  });

  it("handles a generic modal stack push without approval-specific state", () => {
    renderChatPane(root);
    const trigger = openMenu();
    const triggerFocus = vi.spyOn(trigger, "focus");
    const panel = document.createElement("div");
    const button = document.createElement("button");
    panel.append(button);
    document.body.append(panel);
    const token = Symbol("external-modal");
    try {
      act(() => modalStack.push({ token, getPanel: () => panel, getOnClose: () => () => undefined,
        focusInitial: () => button.focus() }));
      expect(hidePopover).toHaveBeenCalled();
      expect(container.querySelector(".th-termhead-overflow")).toBeNull();
      expect(trigger.getAttribute("aria-expanded")).toBe("false");
      expect(document.activeElement).toBe(button);
      expect(triggerFocus).not.toHaveBeenCalled();
    } finally {
      act(() => modalStack.remove(token));
      panel.remove();
    }
  });
});
