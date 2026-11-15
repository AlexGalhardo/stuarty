import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

// A separate file from vite.config.ts: the unit tests do not need the Tailwind,
// Inertia and vukem plugins of the build.
export default defineConfig({
	resolve: {
		alias: {
			"@": fileURLToPath(new URL("./resources/js", import.meta.url)),
		},
	},
	test: {
		include: ["resources/js/**/*.test.{ts,tsx}", "scripts/*.test.ts"],
		environment: "node",
	},
});
