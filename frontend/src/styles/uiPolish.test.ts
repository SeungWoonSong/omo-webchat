import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import postcss from "postcss";

const style = (name: string) => postcss.parse(readFileSync(`src/styles/${name}.css`, "utf8"));
const value = (file: string, selector: string, property: string) => {
  let result = "";
  style(file).walkRules(rule => { if (rule.selectors.includes(selector)) rule.walkDecls(property, decl => { result = decl.value; }); });
  return result;
};

describe("UI polish layout contracts", () => {
  it("fills the prose column with a real table and scrolls only its wrapper", () => {
    expect(value("chat-transcript", ".th-chat-markdown table", "width")).toBe("100%");
    expect(value("chat-transcript", ".th-chat-markdown table", "display")).toBe("");
    expect(value("chat-transcript", ".th-chat-table-wrap", "overflow-x")).toBe("auto");
    expect(value("chat-transcript", ".th-chat-markdown td", "padding")).toBe("var(--th-space-2) var(--th-space-3)");
  });
  it("restores nested lists and muted markers for both message roles", () => {
    expect(value("chat-transcript", ".th-chat-markdown ul", "list-style-type")).toBe("disc");
    expect(value("chat-transcript", ".th-chat-markdown ol", "list-style-type")).toBe("decimal");
    expect(value("chat-transcript", ".th-chat-markdown ul ul", "list-style-type")).toBe("circle");
    expect(value("chat-transcript", ".th-chat-markdown li::marker", "color")).toBe("var(--th-muted)");
  });
  it("keeps right-aligned fit-content bubbles restrained, with an upright steer label", () => {
    expect(value("chat-transcript", ".th-chat-msg--user", "width")).toBe("fit-content");
    expect(value("chat-transcript", ".th-chat-msg", "max-width")).toBe("80%");
    expect(value("chat-transcript", ".th-chat-msg--steer", "font-style")).toBe("normal");
    expect(value("chat-transcript", ".th-chat-msg--steer", "flex-direction")).toBe("column");
  });
  it("unifies settings heights and square steppers at desktop and touch sizes", () => {
    const css = readFileSync("src/styles/settings-menu.css", "utf8");
    expect(css).toContain("--th-settings-control-height: 32px");
    expect(css).toContain("--th-settings-control-height: 44px");
    for (const selector of [".th-settings-seg-btn", ".th-settings-select"]) expect(value("settings-menu", selector, "min-height")).toBe("var(--th-settings-control-height)");
    for (const prop of ["width", "height"]) expect(value("settings-menu", ".th-settings-size-btn", prop)).toBe("var(--th-settings-control-height)");
  });
  it("keeps thinking on one scrolling row and reserves space for the trailing check", () => {
    expect(value("chat-pane", ".th-thinking-in-picker-levels", "flex-wrap")).toBe("nowrap");
    expect(value("chat-pane", ".th-thinking-in-picker-levels", "overflow-x")).toBe("auto");
    expect(value("chat-pane", ".th-model-picker-check", "position")).toBe("absolute");
    expect(value("chat-pane", ".th-model-picker-list > button", "padding")).toContain("var(--th-space-8)");
  });
  it("sizes accounts to content with an internal scrollport", () => {
    expect(value("accounts", ".th-accounts", "height")).toBe("");
    expect(value("accounts", ".th-accounts", "max-height")).toContain("* 80");
    expect(value("accounts", ".th-accounts-body", "overflow-y")).toBe("auto");
  });
});
