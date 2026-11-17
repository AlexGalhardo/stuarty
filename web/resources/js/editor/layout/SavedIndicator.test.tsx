import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { devFixture } from "../boot/devFixture";
import {
	FixtureServer,
	flush,
	ManualTimers,
	makeDocument,
} from "../fixtures/fixtureServer";
import { EditorProvider } from "../state/EditorContext";
import { createRuntime } from "../state/runtime";
import { SavedIndicator } from "./TopBar";

const messages = devFixture(1).bootstrap.messages;

async function render(live: boolean, started: boolean): Promise<string> {
	const server = new FixtureServer(makeDocument({}));
	const runtime = createRuntime({
		projectId: server.projectId,
		actor: "local:ui",
		mode: "local",
		transport: server,
		timers: new ManualTimers(),
	});
	if (started) {
		await runtime.sync.start();
		await flush();
		await runtime.sync.settled();
	}
	return renderToString(
		<I18nProvider messages={messages} locale="en">
			<EditorProvider runtime={runtime}>
				<SavedIndicator live={live} />
			</EditorProvider>
		</I18nProvider>,
	);
}

describe("SavedIndicator", () => {
	test("names the time of the open revision once the project is loaded", async () => {
		expect(await render(true, true)).toMatch(
			/Saved automatically: \d{1,2}:\d{2}/,
		);
	});

	test("says nothing before the first revision or while not live", async () => {
		expect(await render(true, false)).not.toContain("Saved");
		expect(await render(false, true)).not.toContain("Saved");
	});
});
