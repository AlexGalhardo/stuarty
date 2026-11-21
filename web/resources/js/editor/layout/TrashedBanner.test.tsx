import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import { I18nProvider } from "@/lib/i18n";
import { devFixture } from "../boot/devFixture";
import {
	FixtureServer,
	flush,
	ManualTimers,
	makeDocument,
} from "../fixtures/fixtureServer";
import UndoRedo from "../history/UndoRedo";
import { EditorProvider } from "../state/EditorContext";
import { createRuntime } from "../state/runtime";
import TrashedBanner from "./TrashedBanner";

// The attribute, not the disabled: variants of the Tailwind classes.
const isDisabled = (tag: string): boolean => /\sdisabled(=|\s|>)/.test(tag);

const messages = devFixture(1).bootstrap.messages;

async function started(state: "active" | "trashed") {
	const server = new FixtureServer(makeDocument({}));
	server.state = state;
	const runtime = createRuntime({
		projectId: server.projectId,
		actor: "local:ui",
		mode: "local",
		transport: server,
		timers: new ManualTimers(),
	});
	await runtime.sync.start();
	await flush();
	await runtime.sync.settled();
	return runtime;
}

function render(runtime: Awaited<ReturnType<typeof started>>): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en">
			<EditorProvider runtime={runtime}>
				<TooltipProvider>
					<TrashedBanner />
					<UndoRedo />
				</TooltipProvider>
			</EditorProvider>
		</I18nProvider>,
	);
}

describe("a trashed project", () => {
	test("shows the banner and disables Undo and Redo", async () => {
		const html = render(await started("trashed"));
		expect(html).toContain("This project is in the trash");
		const buttons = html.match(/<button[^>]*>/g) ?? [];
		expect(buttons.length).toBeGreaterThan(0);
		expect(buttons.every(isDisabled)).toBe(true);
	});

	test("an active project has no banner and its buttons are enabled", async () => {
		const html = render(await started("active"));
		expect(html).not.toContain("in the trash");
		const enabled = (html.match(/<button[^>]*>/g) ?? []).filter(
			(b) => !isDisabled(b),
		);
		expect(enabled.length).toBeGreaterThan(0);
	});
});
