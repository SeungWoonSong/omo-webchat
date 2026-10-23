import { act } from "react";
import type { ReactNode } from "react";
import { createRoot } from "react-dom/client";
import type { Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useAppConfig } from "../../app-config";
import { SettingsMenu } from "../../components/SettingsMenu";
import { I18nContext } from "../../i18n";
import { persistTheme } from "../../lib/theme";
import { readCollapseSystemNotices, persistCollapseSystemNotices } from "../../lib/noticePreferences";
import { HookCard } from "./HookCard";
import { TranscriptNoticeRow } from "./TranscriptNoticeRow";
import { SummaryNoticeBox } from "./SummaryNoticeBox";

let container: HTMLDivElement;
let root: Root;

function Harness({ children }: { readonly children: ReactNode }) {
  const config = useAppConfig();
  return (
    <I18nContext.Provider value={config}>
      <SettingsMenu onOpenStats={() => undefined} onOpenEngineRestart={() => undefined} />
      {children}
    </I18nContext.Provider>
  );
}

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  window.localStorage.clear();
  persistTheme("light");
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  window.localStorage.clear();
  vi.unstubAllGlobals();
});

function render(children: ReactNode): void {
  act(() => root.render(<Harness>{children}</Harness>));
}

it.each(["info", "profile", "summary"])("collapses %s by default and expands its full text on click", (kind) => {
  const text = "NOTICE_BODY_SENTINEL\nSECOND_LINE_SENTINEL";
  render(kind === "profile"
    ? <HookCard hookType="omo-model-profile:unavailable" text={text} />
    : kind === "summary"
      ? <SummaryNoticeBox label="[branch]" summary={text} />
      : <TranscriptNoticeRow notice={{ id: 1, at: 0, kind: "engine_notify", payload: { message: text, notifyType: "info" } }} />);
  const toggle = container.querySelector<HTMLButtonElement>(".th-notice-disclosure-toggle");
  expect(toggle?.getAttribute("aria-expanded")).toBe("false");
  expect(container.textContent).not.toContain(text);
  act(() => toggle?.click());
  expect(toggle?.getAttribute("aria-expanded")).toBe("true");
  expect(container.textContent).toContain(text);
  const details = container.querySelector("details");
  if (details) expect(details.open).toBe(true);
  act(() => toggle?.click());
  expect(container.textContent).not.toContain(text);
});

it("updates mounted notices from Settings and persists OFF across remounts", () => {
  const text = "MODEL_PROFILE_SENTINEL";
  const content = <HookCard hookType="omo-model-profile:unavailable" text={text} />;
  render(content);
  act(() => container.querySelector<HTMLButtonElement>(".th-settings-menu > button")?.click());
  const toggle = container.querySelector<HTMLButtonElement>('[role="switch"]');
  expect(toggle?.getAttribute("aria-checked")).toBe("true");
  act(() => toggle?.click());
  expect(toggle?.getAttribute("aria-checked")).toBe("false");
  expect(readCollapseSystemNotices()).toBe(false);
  expect(container.querySelector(".th-notice-disclosure-toggle")).toBeNull();
  expect(container.textContent).toContain(text);
  act(() => root.unmount());
  root = createRoot(container);
  render(content);
  expect(container.textContent).toContain(text);
  expect(container.querySelector(".th-notice-disclosure-toggle")).toBeNull();
});

it("renders the full info notice when the persisted preference is OFF", () => {
  persistCollapseSystemNotices(false);
  render(<TranscriptNoticeRow notice={{ id: 1, at: 0, kind: "extension_notify", payload: { message: "INFO_SENTINEL" } }} />);
  expect(container.textContent).toContain("INFO_SENTINEL");
  expect(container.querySelector(".th-notice-disclosure-toggle")).toBeNull();
});

it("never collapses a summary carrying warning severity", () => {
  render(<TranscriptNoticeRow notice={{ id: 1, at: 0, kind: "compaction_summary", payload: { severity: "warning", summary: "SUMMARY_WARNING_SENTINEL" } }} />);
  expect(container.textContent).toContain("SUMMARY_WARNING_SENTINEL");
  expect(container.querySelector("details")?.open).toBe(true);
  expect(container.querySelector(".th-notice-disclosure-toggle")).toBeNull();
});

it.each([
  ["engine_notify", { notifyType: "error" }],
  ["engine_notify", { notifyType: "warning" }],
  ["extension_notify", { severity: "warning" }],
  ["extension_notify", { level: "error" }],
  ["extension_notify", { type: "warning" }],
  ["retry_fallback_exhausted", {}],
  ["server_fallback_aborted", {}],
  ["continuation_error", {}],
  ["auto_retry_end", { success: false }],
  ["high_reasoning_warning", {}],
  ["cache_miss", {}],
] as const)("keeps %s diagnostics fully visible with collapse ON", (kind, payload) => {
  render(<TranscriptNoticeRow notice={{ id: 1, at: 0, kind, payload: { ...payload, message: "ERROR_SENTINEL", text: "ERROR_SENTINEL" } }} />);
  expect(container.textContent).toContain("ERROR_SENTINEL");
  expect(container.querySelector(".th-notice-disclosure-toggle")).toBeNull();
});
