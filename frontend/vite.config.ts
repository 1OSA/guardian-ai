import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

// Read the version from package.json so there is a single source of truth.
// The value is injected as __APP_VERSION__ in the client bundle AND can be
// passed to the Go binary via -ldflags at build time:
//   go build -ldflags "-X main.AppVersion=$(node -p "require('./frontend/package.json').version")" .
const pkg = JSON.parse(
  readFileSync(resolve(__dirname, "package.json"), "utf-8"),
) as { version: string };

// https://vite.dev/config/
export default defineConfig({
  define: {
    // Exposes the version as a compile-time constant in the React app.
    // Usage: declare const __APP_VERSION__: string;
    __APP_VERSION__: JSON.stringify(pkg.version),
  },
  plugins: [react(), tailwindcss()],
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          "vendor-react": ["react", "react-dom", "react/jsx-runtime"],
          "vendor-router": ["react-router-dom"],
          "vendor-charts": ["recharts"],
          "vendor-icons": ["react-icons"],
        },
      },
    },
  },
});
