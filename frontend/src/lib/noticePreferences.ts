/** Fixed, fork-wide preference. Missing or invalid values default to collapsed. */
export const NOTICE_COLLAPSE_STORAGE_KEY = "th-collapse-system-notices";

export function readCollapseSystemNotices(): boolean {
  try {
    return window.localStorage.getItem(NOTICE_COLLAPSE_STORAGE_KEY) !== "false";
  } catch {
    return true;
  }
}

export function persistCollapseSystemNotices(collapsed: boolean): void {
  try {
    window.localStorage.setItem(NOTICE_COLLAPSE_STORAGE_KEY, String(collapsed));
  } catch {
    // Like theme preferences, private-mode storage failures keep the in-memory choice.
  }
}
