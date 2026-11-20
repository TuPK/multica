import { LANGUAGES, setLanguage, t, useLanguage, type Language } from "../i18n";
import { THEMES, setTheme, useTheme, type Theme } from "../theme";

// Language codes are shown as they are; they are not translated.
const LANGUAGE_LABELS: Record<Language, string> = { en: "EN", vi: "VI" };

function themeLabel(theme: Theme): string {
  switch (theme) {
    case "blue-white":
      return t("Blue-white");
    case "light":
      return t("Light");
    case "dark":
      return t("Dark");
  }
}

export function LanguageSwitch() {
  const language = useLanguage();
  return (
    <div
      role="group"
      aria-label={t("Language")}
      className="inline-flex shrink-0 rounded-md border border-surface-border p-0.5"
    >
      {LANGUAGES.map((code) => {
        const selected = code === language;
        return (
          <button
            key={code}
            type="button"
            aria-pressed={selected}
            onClick={() => setLanguage(code)}
            className={
              "rounded px-2 py-0.5 text-label " +
              (selected
                ? "bg-surface-selected text-foreground"
                : "text-muted-foreground hover:text-foreground")
            }
          >
            {LANGUAGE_LABELS[code]}
          </button>
        );
      })}
    </div>
  );
}

export function ThemeSwitch() {
  const theme = useTheme();
  return (
    <div
      role="group"
      aria-label={t("Theme")}
      className="inline-flex shrink-0 rounded-md border border-surface-border p-0.5"
    >
      {THEMES.map((option) => {
        const selected = option === theme;
        return (
          <button
            key={option}
            type="button"
            aria-pressed={selected}
            onClick={() => setTheme(option)}
            className={
              "rounded px-2 py-0.5 text-label " +
              (selected
                ? "bg-surface-selected text-foreground"
                : "text-muted-foreground hover:text-foreground")
            }
          >
            {themeLabel(option)}
          </button>
        );
      })}
    </div>
  );
}
