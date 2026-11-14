import type { Message } from "@/lib/i18n";
import en from "../../../lang/en.json";
import { FixtureServer, makeDocument } from "../fixtures/fixtureServer";
import type { RuntimeOptions } from "../state/runtime";
import type { EditorBootstrap } from "./bootstrap";

// Development only: `?fixture=<items per track>` opens the editor on a
// FixtureServer instead of the HTTP API, so the shell and the timeline can be
// seen and measured before (or without) a server. editor.tsx imports this
// behind import.meta.env.DEV, so the production bundle does not contain it.

const PLURAL_CATEGORIES = new Set([
	"zero",
	"one",
	"two",
	"few",
	"many",
	"other",
]);

// Flattens the nested catalog the way the server does: keys joined with ".",
// a map whose keys are all plural categories stays one message.
function flatten(
	node: Record<string, unknown>,
	prefix: string,
	out: Record<string, Message>,
): void {
	for (const [key, value] of Object.entries(node)) {
		const path = prefix === "" ? key : `${prefix}.${key}`;
		if (typeof value === "string") out[path] = value;
		else if (typeof value === "object" && value !== null) {
			const map = value as Record<string, unknown>;
			if (Object.keys(map).every((k) => PLURAL_CATEGORIES.has(k))) {
				out[path] = map as Record<string, string>;
			} else flatten(map, path, out);
		}
	}
}

export function devFixture(itemsPerTrack: number): {
	bootstrap: EditorBootstrap;
	options: Pick<RuntimeOptions, "transport" | "checked">;
	server: FixtureServer;
} {
	const server = new FixtureServer(
		makeDocument({ tracks: 5, itemsPerTrack, itemFrames: 40, gap: 10 }),
	);
	const messages: Record<string, Message> = {};
	flatten(en as Record<string, unknown>, "", messages);
	return {
		bootstrap: {
			project_id: server.projectId,
			mode: "local",
			api_base: "/api/v1",
			locale: "en",
			locales: ["en"],
			messages,
			user: null,
			phase: "prototype",
			version: "dev-fixture",
		},
		options: { transport: server, checked: true },
		server,
	};
}
