import { useCallback, useContext, useEffect, useMemo, useState } from "react";
import {
  detectLang,
  I18nContext,
  persistLang,
  translate,
  useDocumentLang,
} from "./i18n";
import type { I18nValue, Lang } from "./i18n";
import {
  detectFont,
  detectFontSize,
  FONT_PRESETS,
  persistFont,
  persistFontSize,
  SYSTEM_FONT_STACK,
} from "./lib/font";
import type { FontId } from "./lib/font";
import {
  applyTheme,
  detectTheme,
  persistTheme,
  resolveTheme,
  watchSystemTheme,
} from "./lib/theme";
import type { ThemeId } from "./lib/theme";
import { readCollapseSystemNotices, persistCollapseSystemNotices } from "./lib/noticePreferences";

export interface NoticePreference {
  readonly collapseSystemNotices: boolean;
  readonly setCollapseSystemNotices: (collapsed: boolean) => void;
}

export interface ThemePreference {
  readonly theme: ThemeId;
  readonly setTheme: (theme: ThemeId) => void;
}

export type AppConfigValue = I18nValue & ThemePreference & NoticePreference;

export function useAppConfig(): AppConfigValue {
  const [collapseSystemNotices, setCollapseState] = useState(readCollapseSystemNotices);
  const setCollapseSystemNotices = useCallback((next: boolean) => {
    setCollapseState(next);
    persistCollapseSystemNotices(next);
  }, []);
  const [lang, setLangState] = useState<Lang>(detectLang);
  useDocumentLang(lang);
  const setLang = useCallback((next: Lang) => {
    setLangState(next);
    persistLang(next);
  }, []);
  const [font, setFontState] = useState<FontId>(detectFont);
  const setFont = useCallback((next: FontId) => {
    setFontState(next);
    persistFont(next);
  }, []);
  const [fontSize, setFontSizeState] = useState(detectFontSize);
  const setFontSize = useCallback((next: number) => {
    setFontSizeState(next);
    persistFontSize(next);
  }, []);
  const [theme, setThemeState] = useState<ThemeId>(detectTheme);
  const setTheme = useCallback((next: ThemeId) => {
    setThemeState(next);
    persistTheme(next);
  }, []);
  const t = useCallback(
    (key: string, vars?: Readonly<Record<string, string | number>>) => translate(lang, key, vars),
    [lang],
  );

  useEffect(() => {
    document.title = t("app.title");
  }, [t]);

  useEffect(() => {
    const preset = FONT_PRESETS.find((candidate) => candidate.id === font);
    const stack = preset ? preset.stack : SYSTEM_FONT_STACK;
    document.documentElement.style.setProperty("--th-font-mono", stack);
    document.documentElement.style.setProperty("--th-font-size", `${fontSize}px`);
  }, [font, fontSize]);

  // Apply the resolved theme (index.html applies the same precedence before
  // hydration) and, while the choice is "system", follow live OS changes.
  useEffect(() => {
    applyTheme(resolveTheme(theme));
    if (theme !== "system") return undefined;
    return watchSystemTheme(applyTheme);
  }, [theme]);

  return useMemo(
    () => ({ lang, setLang, font, setFont, fontSize, setFontSize, theme, setTheme, t, collapseSystemNotices, setCollapseSystemNotices }),
    [lang, setLang, font, setFont, fontSize, setFontSize, theme, setTheme, t, collapseSystemNotices, setCollapseSystemNotices],
  );
}

/** Bare i18n consumers retain their original rendering, like the theme slice. */
export function useNoticePreference(): NoticePreference {
  const value = useContext(I18nContext) as I18nValue & Partial<NoticePreference>;
  return {
    collapseSystemNotices: value.collapseSystemNotices ?? false,
    setCollapseSystemNotices: value.setCollapseSystemNotices ?? (() => undefined),
  };
}

/**
 * Reads the theme slice of the app config. App.tsx provides the useAppConfig
 * result through I18nContext, so the theme fields ride along on that value;
 * a bare I18nValue without them (e.g. a hand-rolled context in a test)
 * yields the inert defaults.
 */
export function useTheme(): ThemePreference {
  const value = useContext(I18nContext) as I18nValue & Partial<ThemePreference>;
  return {
    theme: value.theme ?? "system",
    setTheme: value.setTheme ?? (() => undefined),
  };
}
