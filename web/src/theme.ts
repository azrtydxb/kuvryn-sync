import { useCallback, useEffect, useState } from "react";

/** The Azrty colour themes the console supports. */
export type Theme = "dark" | "light";

const KEY = "ksync.theme";

/** The stored theme, dark unless the viewer chose light. */
export function storedTheme(): Theme {
  try {
    return localStorage.getItem(KEY) === "light" ? "light" : "dark";
  } catch {
    return "dark";
  }
}

/** Sets data-theme on <html>, which the Azrty tokens key on. */
export function applyTheme(theme: Theme): void {
  document.documentElement.dataset.theme = theme;
}

/** The current theme and a setter that remembers the choice. */
export function useTheme(): [Theme, (t: Theme) => void] {
  const [theme, setThemeState] = useState<Theme>(storedTheme);
  useEffect(() => applyTheme(theme), [theme]);
  const setTheme = useCallback((t: Theme) => {
    try {
      localStorage.setItem(KEY, t);
    } catch {
      // Private windows may refuse storage; the choice lasts for this page.
    }
    setThemeState(t);
  }, []);
  return [theme, setTheme];
}
