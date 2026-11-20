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
    <select
      aria-label={t("Theme")}
      value={theme}
      onChange={(event) => setTheme(event.target.value as Theme)}
      className="h-8 shrink-0 rounded-md border border-surface-border bg-surface px-2 text-label text-foreground"
    >
      {THEMES.map((option) => (
        <option key={option} value={option}>
          {themeLabel(option)}
        </option>
      ))}
    </select>
  );
}
