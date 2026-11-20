import { SegmentedControl, SegmentedControlItem } from "@astryxdesign/core/SegmentedControl";
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
    <SegmentedControl
      size="sm"
      label={t("Language")}
      value={language}
      onChange={(value) => setLanguage(value as Language)}
    >
      {LANGUAGES.map((code) => (
        <SegmentedControlItem key={code} value={code} label={LANGUAGE_LABELS[code]} />
      ))}
    </SegmentedControl>
  );
}

export function ThemeSwitch() {
  const theme = useTheme();
  return (
    <SegmentedControl
      size="sm"
      label={t("Theme")}
      value={theme}
      onChange={(value) => setTheme(value as Theme)}
    >
      {THEMES.map((option) => (
        <SegmentedControlItem key={option} value={option} label={themeLabel(option)} />
      ))}
    </SegmentedControl>
  );
}
