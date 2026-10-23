import { act } from "react";
import { createRoot } from "react-dom/client";
import type { Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AccountsPanel } from "./AccountsPanel";
import { I18nContext } from "../i18n";
import { useAppConfig } from "../app-config";
import * as accounts from "../lib/accounts";
import type { LoginEvent } from "../lib/accounts";

vi.mock("../lib/accounts", async (original) => ({
  ...await original<typeof accounts>(),
  listAccountProviders: vi.fn(),
  startAccountLogin: vi.fn(),
  cancelAccountLogin: vi.fn(),
  answerAccountLogin: vi.fn(),
  accountLoginEvents: vi.fn(),
  changeAccount: vi.fn(),
}));

let root: Root;
let host: HTMLDivElement;
let receive: (event: LoginEvent) => void;
let fail: () => void;
const stop = vi.fn();

function Harness() {
  const config = useAppConfig();
  return <I18nContext.Provider value={config}><AccountsPanel onClose={() => undefined} /></I18nContext.Provider>;
}
function button(text: string): HTMLButtonElement {
  const b = Array.from(document.querySelectorAll("button")).find((b) => b.textContent === text);
  if (!b) throw new Error(`button missing: ${text}`);
  return b;
}
async function render() { await act(async () => root.render(<Harness />)); }
async function start() { await act(async () => button("Add account").click()); }
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  window.localStorage.setItem("th-lang","en");
  vi.spyOn(window,"open").mockReturnValue(null);
  vi.mocked(accounts.listAccountProviders).mockResolvedValue({providers:[{id:"cursor",name:"Cursor",status:{configured:false},accounts:[]}]});
  vi.mocked(accounts.startAccountLogin).mockResolvedValue({id:"flow"});
  vi.mocked(accounts.cancelAccountLogin).mockResolvedValue(undefined);
  vi.mocked(accounts.answerAccountLogin).mockResolvedValue(undefined);
  vi.mocked(accounts.changeAccount).mockResolvedValue(undefined);
  vi.mocked(accounts.accountLoginEvents).mockImplementation((_id, r, f) => { receive=r; fail=f; return stop; });
  host = document.createElement("div"); document.body.append(host); root=createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove(); window.localStorage.clear(); vi.restoreAllMocks(); vi.clearAllMocks(); vi.unstubAllGlobals();
});

it("shows provider status and Cursor waiting with a copyable real link", async () => {
  await render(); await start();
  await act(async () => receive({type:"auth_login_url",url:"cursor://auth/callback?state=example"}));
  expect(document.querySelector<HTMLInputElement>('[aria-label="Login URL"]')?.value).toBe("cursor://auth/callback?state=example");
  expect(button("Add account").disabled).toBe(true);
  expect(document.querySelector(".th-accounts-flow [role=status]")).not.toBeNull();
});
it("relays choice, code input and confirm without chat transport", async () => {
  await render(); await start();
  await act(async () => receive({type:"extension_ui_request",id:"choice",method:"select",options:["Device code"]}));
  await act(async () => button("Device code").click());
  expect(accounts.answerAccountLogin).toHaveBeenLastCalledWith("flow",{id:"choice",value:"Device code"});
  await act(async () => receive({type:"extension_ui_request",id:"code",method:"input",title:"Device code: ABCD-1234"}));
  const input=document.querySelector<HTMLInputElement>("#accounts-code");
  if (!input) throw new Error("paste input missing");
  const setter=Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,"value")?.set;
  await act(async () => { setter?.call(input,"redirect-code"); input.dispatchEvent(new Event("input",{bubbles:true})); });
  await act(async () => document.querySelector("form")?.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true})));
  expect(accounts.answerAccountLogin).toHaveBeenLastCalledWith("flow",{id:"code",value:"redirect-code"});
  expect(document.querySelector("#accounts-code")).toBeNull();
  await act(async () => receive({type:"extension_ui_request",id:"confirm",method:"confirm"}));
  await act(async () => button("Yes").click());
  expect(accounts.answerAccountLogin).toHaveBeenLastCalledWith("flow",{id:"confirm",confirmed:true});
});
it("cancels and clears private flow data on end", async () => {
  await render(); await start();
  await act(async () => button("Cancel login").click());
  expect(accounts.cancelAccountLogin).toHaveBeenCalledWith("flow");
  await act(async () => receive({type:"auth_login_end",success:false}));
  expect(document.querySelector("#accounts-code")).toBeNull();
  expect(document.querySelector(".th-accounts-flow a")).toBeNull();
  expect(stop).toHaveBeenCalled();
  expect(button("Add account").disabled).toBe(false);
});
it("refreshes after success and offers retry after stream failure", async () => {
  await render(); await start();
  await act(async () => receive({type:"auth_login_end",success:true}));
  expect(accounts.listAccountProviders).toHaveBeenCalledTimes(2);
  await start();
  await act(async () => fail());
  expect(document.querySelector("[role=alert]")).not.toBeNull();
  expect(button("Add account").disabled).toBe(false);
});
it("aborts its private stream on unmount", async () => {
  await render(); await start();
  await act(async () => root.render(null));
  expect(stop).toHaveBeenCalledTimes(1);
});
it("renders the Korean account panel", async () => {
  window.localStorage.setItem("th-lang","ko");
  await render();
  expect(button("계정 추가")).toBeDefined();
});

function connected() {
  vi.mocked(accounts.listAccountProviders).mockResolvedValue({ providers: [{
    id: "cursor", name: "Cursor", status: { configured: true },
    accounts: [{ name: "immutable-id", displayName: "work@example.test", blocked: false, pinned: false, source: "login" }],
  }] });
}
it("renders connected account descriptors and keeps removal behind a cancelable confirmation", async () => {
  connected();
  await render();
  expect(document.querySelector(".th-accounts-badge")?.getAttribute("data-status")).toBe("connected");
  await act(async () => button("Remove account").click());
  expect(document.querySelectorAll('[role="dialog"]').length).toBe(2);
  expect(document.querySelector(".th-confirm-message")?.textContent).toContain("work@example.test");
  await act(async () => button("Keep account").click());
  expect(accounts.changeAccount).not.toHaveBeenCalled();
});
it("removes only the confirmed immutable account and refreshes the list", async () => {
  connected(); await render();
  await act(async () => button("Remove account").click());
  await act(async () => document.querySelector<HTMLButtonElement>(".th-confirm .th-btn--danger")?.click());
  expect(accounts.changeAccount).toHaveBeenCalledWith("cursor", "remove", "immutable-id");
  expect(accounts.listAccountProviders).toHaveBeenCalledTimes(2);
});
it("preserves accounts when removal fails and shows an actionable error", async () => {
  connected(); vi.mocked(accounts.changeAccount).mockRejectedValue(new Error("private engine text"));
  await render();
  await act(async () => button("Remove account").click());
  await act(async () => document.querySelector<HTMLButtonElement>(".th-confirm .th-btn--danger")?.click());
  expect(document.querySelector("[role=alert]")).not.toBeNull();
  expect(document.body.textContent).not.toContain("private engine text");
  expect(document.querySelector(".th-accounts-identity")).not.toBeNull();
});
it("pins the selected account through the engine", async () => {
  connected(); await render();
  await act(async () => button("Set preferred").click());
  expect(accounts.changeAccount).toHaveBeenCalledWith("cursor", "pin", "immutable-id");
});
it("shows empty provider results without inventing connected accounts", async () => {
  vi.mocked(accounts.listAccountProviders).mockResolvedValue({ providers: [] });
  await render();
  expect(document.querySelectorAll(".th-accounts-list > li")).toHaveLength(0);
  expect(document.querySelectorAll(".th-accounts-badge")).toHaveLength(0);
});
it("shows the active step and focuses the code input only when the engine requests input", async () => {
  await render(); await start();
  expect(document.querySelector("#accounts-code")).toBeNull();
  expect(document.querySelector('[aria-current="step"] span')?.textContent).toBe("1");
  await act(async () => receive({ type: "extension_ui_request", method: "input", id: "code" }));
  expect(document.activeElement?.id).toBe("accounts-code");
  expect(document.querySelector('[aria-current="step"] span')?.textContent).toBe("2");
  await act(async () => receive({ type: "auth_login_end", success: true }));
  expect(document.querySelector('[aria-current="step"] span')?.textContent).toBe("3");
  expect(document.querySelector("#accounts-code")).toBeNull();
});
it("shows a localized error and supports retry when loading fails", async () => {
  vi.mocked(accounts.listAccountProviders).mockRejectedValueOnce(new Error("unavailable"));
  await render();
  expect(document.querySelector("[role=alert]")).not.toBeNull();
  await act(async () => button("Refresh status").click());
  expect(document.querySelector("[role=alert]")).toBeNull();
});
it("cancels a pending login setup and cancels its eventual engine flow", async () => {
  let ready: ((flow: { readonly id: string }) => void) | undefined;
  vi.mocked(accounts.startAccountLogin).mockImplementation(() => new Promise((resolve) => { ready = resolve; }));
  await render(); await start();
  await act(async () => button("Cancel login").click());
  expect(button("Add account").disabled).toBe(false);
  await act(async () => ready?.({ id: "late-flow" }));
  expect(accounts.cancelAccountLogin).toHaveBeenCalledWith("late-flow");
  expect(accounts.accountLoginEvents).not.toHaveBeenCalled();
});
it("keeps the login open with an error when cancellation fails", async () => {
  vi.mocked(accounts.cancelAccountLogin).mockRejectedValueOnce(new Error("failed"));
  await render(); await start();
  await act(async () => button("Cancel login").click());
  expect(document.querySelector("[role=alert]")).not.toBeNull();
  expect(button("Add account").disabled).toBe(true);
  await act(async () => button("Cancel login").click());
  expect(button("Add account").disabled).toBe(false);
});
it("restores automatic selection by clearing a preferred account", async () => {
  vi.mocked(accounts.listAccountProviders).mockResolvedValue({ providers: [{
    id: "cursor", name: "Cursor", status: { configured: true },
    accounts: [{ name: "work", blocked: false, pinned: true }],
  }] });
  await render();
  await act(async () => button("Select automatically").click());
  expect(accounts.changeAccount).toHaveBeenCalledWith("cursor", "pin", null);
});
it("reports engine login failure without exposing private engine text", async () => {
  await render(); await start();
  await act(async () => receive({ type: "auth_login_end", success: false, error: "private login details" }));
  expect(document.querySelector("[role=alert]")).not.toBeNull();
  expect(document.querySelector(".th-accounts-badge")?.getAttribute("data-status")).toBe("error");
  expect(document.body.textContent).not.toContain("private login details");
});
