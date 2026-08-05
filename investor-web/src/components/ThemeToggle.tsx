import { useTheme } from "@/hooks/useTheme";

/** Dark default; toggles data-theme="light" with localStorage persistence. */
export function ThemeToggle() {
  const { theme, toggleTheme } = useTheme();
  const next = theme === "dark" ? "light" : "dark";

  return (
    <button
      type="button"
      className="theme-toggle"
      onClick={toggleTheme}
      aria-label={`Switch to ${next} theme`}
      title={`Switch to ${next} theme`}
    >
      {theme === "dark" ? "Light" : "Dark"}
    </button>
  );
}
