import React, { useState, useEffect } from "react";
import { FaSun, FaMoon } from "react-icons/fa";

const ThemeToggle: React.FC = () => {
  const [theme, setTheme] = useState<"dark" | "light">(() => {
    if (typeof window === "undefined") return "dark";
    const stored = localStorage.getItem("guardian-theme");
    if (stored === "light" || stored === "dark") return stored;
    return window.matchMedia?.("(prefers-color-scheme: light)").matches
      ? "light"
      : "dark";
  });

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("guardian-theme", theme);
  }, [theme]);

  const toggleTheme = () => {
    setTheme((prev) => (prev === "dark" ? "light" : "dark"));
  };

  return (
    <button
      onClick={toggleTheme}
      className="flex items-center gap-1.5 px-2.5 py-1 rounded-md border border-border bg-surface-1 text-text-muted text-[11px] font-semibold cursor-pointer hover:bg-surface-2 transition-colors"
      title={`Switch to ${theme === "dark" ? "light" : "dark"} mode`}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 6,
        padding: "4px 10px",
        borderRadius: 6,
        border: "1px solid var(--color-border)",
        background: "var(--color-surface-1)",
        color: "var(--color-text-muted)",
        fontSize: 11,
        fontWeight: 600,
        cursor: "pointer",
        minWidth: 70,
        justifyContent: "center",
      }}
    >
      {theme === "dark" ? (
        <FaSun className="text-yellow-500" />
      ) : (
        <FaMoon className="text-indigo-400" />
      )}
    </button>
  );
};

export default ThemeToggle;
