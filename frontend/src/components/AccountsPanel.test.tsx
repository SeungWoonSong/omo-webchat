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
async function start() { await act(async () => button("Add / login").click()); }
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  window.localStorage.setItem("th-lang","en");
  vi.spyOn(window,"open").mockReturnValue(null);
  vi.mocked(accounts.listAccountProviders).mockResolvedValue({providers:[{id:"cursor",name:"Cursor",status:{configured:false},accounts:[]}]});
  vi.mocked(accounts.startAccountLogin).mockResolvedValue({id:"flow"});
  vi.mocked(accounts.cancelAccountLogin).mockResolvedValue(undefined);
  vi.mocked(accounts.answerAccountLogin).mockResolvedValue(undefined);
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
  expect(button("Add / login").disabled).toBe(true);
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
  expect(document.querySelector(".th-accounts-flow")).toBeNull();
  expect(stop).toHaveBeenCalled();
  expect(button("Add / login").disabled).toBe(false);
});
it("refreshes after success and offers retry after stream failure", async () => {
  await render(); await start();
  await act(async () => receive({type:"auth_login_end",success:true}));
  expect(accounts.listAccountProviders).toHaveBeenCalledTimes(2);
  await start();
  await act(async () => fail());
  expect(document.querySelector("[role=alert]")).not.toBeNull();
  expect(button("Add / login").disabled).toBe(false);
});
it("aborts its private stream on unmount", async () => {
  await render(); await start();
  await act(async () => root.render(null));
  expect(stop).toHaveBeenCalledTimes(1);
});
it("renders the Korean account panel", async () => {
  window.localStorage.setItem("th-lang","ko");
  await render();
  expect(button("추가 / 로그인")).toBeDefined();
});
