import type { Change } from "../api/types";
import { createRuntime, type EditorRuntime } from "../boot/runtime";
import {
	type DocumentSpec,
	FixtureServer,
	flush,
	ManualTimers,
	makeDocument,
} from "./fixtureServer";

export interface Harness {
	server: FixtureServer;
	timers: ManualTimers;
	runtime: EditorRuntime;
	mismatches: number[];
}

// A runtime wired to a FixtureServer and fake timers, started and settled.
export async function startHarness(
	spec: DocumentSpec = {},
	mode: "local" | "network" = "local",
): Promise<Harness> {
	const server = new FixtureServer(makeDocument(spec));
	const timers = new ManualTimers();
	const mismatches: number[] = [];
	const runtime = createRuntime({
		bootstrap: {
			project_id: server.projectId,
			mode,
			api_base: "/api/v1",
			locale: "en",
			locales: ["en"],
			messages: {},
			user: mode === "network" ? { id: "12", name: "Ana" } : null,
			phase: "prototype",
			version: "test",
		},
		transport: server,
		timers,
		checked: true,
		onMismatch: (revision) => mismatches.push(revision),
	});
	await runtime.sync.start();
	await flush();
	await runtime.sync.settled();
	return { server, timers, runtime, mismatches };
}

export function itemMoved(id: string, before: number, after: number): Change {
	return { kind: "item.updated", id, changes: { start: [before, after] } };
}
