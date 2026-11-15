import { fileURLToPath } from "node:url";
import inertia from "@inertiajs/vite";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

import vukem from "./resources/vite/vukem.js";

// `vite build --mode e2e` is the end-to-end build: it turns on the test hooks of
// SDD-012 section 20.4 that change the project. A .env file would do it, but the
// repository ignores .env.*.
export default defineConfig(({ mode }) => ({
  define: { "import.meta.env.VITE_E2E": JSON.stringify(mode === "e2e" ? "1" : "") },
  resolve: { alias: { "@": fileURLToPath(new URL("./resources/js", import.meta.url)) } },
  plugins: [
    tailwindcss(),
    react(),
    inertia(),
    vukem({
      input: ["resources/css/app.css", "resources/js/app.tsx", "resources/js/editor.tsx"],
    }),
  ],
}));
