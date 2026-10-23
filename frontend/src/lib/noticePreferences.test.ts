import { afterEach, describe, expect, it, vi } from "vitest";
import { NOTICE_COLLAPSE_STORAGE_KEY, persistCollapseSystemNotices, readCollapseSystemNotices } from "./noticePreferences";

afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
});

describe("notice preferences", () => {
  it.each([null, "", "invalid", "true"])("defaults to ON for %s", (stored) => {
    if (stored !== null) window.localStorage.setItem(NOTICE_COLLAPSE_STORAGE_KEY, stored);
    expect(readCollapseSystemNotices()).toBe(true);
  });

  it.each([false, true])("persists and reads %s at the fixed key", (collapsed) => {
    persistCollapseSystemNotices(collapsed);
    expect(window.localStorage.getItem(NOTICE_COLLAPSE_STORAGE_KEY)).toBe(String(collapsed));
    expect(readCollapseSystemNotices()).toBe(collapsed);
  });

  it("defaults to ON when reading storage is unavailable", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new DOMException("unavailable"); });
    expect(readCollapseSystemNotices()).toBe(true);
  });

  it("allows an in-memory choice when writing storage is unavailable", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new DOMException("unavailable"); });
    expect(() => persistCollapseSystemNotices(false)).not.toThrow();
  });
});
