import { readFileSync } from "node:fs";
import postcss from "postcss";
import { describe, expect, it } from "vitest";

describe("chat-pane header overflow touch targets", () => {
  it("sets a 44px floor on compact triggers and actions without changing desktop density", () => {
    const css = postcss.parse(readFileSync("src/styles/chat-pane.css", "utf8"));
    const compact = css.nodes.find(node => node.type === "atrule"
      && node.name === "container" && node.params === "chat-pane (max-width: 600px)");
    expect(compact).toBeDefined();
    for (const selector of [
      ".th-chat-pane .th-termhead-more-btn",
      ".th-chat-pane .th-termhead-overflow .th-tree-overflow-item",
    ]) {
      const floors: { value: string; parent: unknown }[] = [];
      css.walkRules(rule => {
        if (!rule.selectors.includes(selector)) return;
        rule.walkDecls("min-height", declaration => {
          floors.push({ value: declaration.value, parent: rule.parent });
        });
      });
      expect(floors).toHaveLength(1);
      expect(floors[0]?.value).toBe("44px");
      expect(floors[0]?.parent).toBe(compact);
    }
  });
});
