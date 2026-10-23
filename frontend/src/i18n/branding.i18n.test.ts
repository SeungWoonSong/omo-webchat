import { describe, expect, it } from "vitest";
import en from "./locales/en.json";
import ko from "./locales/ko.json";

// Internal protocol/provider identifiers are not locale values.
describe("user-visible product branding", () => {
  for (const [lang, messages] of Object.entries({ en, ko })) {
    it(`${lang} contains no legacy product branding`, () => {
      expect(Object.entries(messages).filter(([, value]) => /\bomo\b/i.test(value))).toEqual([]);
    });
    it(`${lang} has exactly one editable product-name key`, () => {
      expect(messages["app.name"]).toBe(lang === "ko" ? "AI 작업실" : "AI Workspace");
      expect(Object.entries(messages).filter(([, value]) => value === messages["app.name"]).map(([key]) => key)).toEqual(["app.name"]);
    });
  }
});
