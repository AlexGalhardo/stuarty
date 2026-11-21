import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { SessionProvider } from "@/lib/session";
import en from "../../../lang/en.json";
import type { AgentProfileListResult } from "../../editor/api/types";
import ProfileSetup from "./ProfileSetup";

function flat(
	tree: Record<string, unknown>,
	prefix: string,
	out: Record<string, string> = {},
): Record<string, string> {
	for (const [key, value] of Object.entries(tree)) {
		if (typeof value === "string") out[`${prefix}${key}`] = value;
		else flat(value as Record<string, unknown>, `${prefix}${key}.`, out);
	}
	return out;
}
const messages = flat(en.ui, "ui.");

const suggestion = {
	name: "opencode",
	title: "OpenCode",
	executables: ["opencode"],
	args: ["acp"],
	install: "Install OpenCode, then sign in to it the way you always do.",
	found: true,
	found_path: "/usr/bin/opencode",
};

function draw(
	result: Partial<AgentProfileListResult>,
	privileged = true,
): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en", "pt-BR"]}>
			<SessionProvider session={{ privileged, mode: "local" }}>
				<ProfileSetup
					profiles={{
						profiles: [],
						suggestions: [suggestion],
						can_write: false,
						available: true,
						...result,
					}}
					busy={false}
					failure={null}
					onSave={async () => true}
					onRemove={() => {}}
				/>
			</SessionProvider>
		</I18nProvider>,
	);
}

describe("ProfileSetup", () => {
	test("with no agent it says which agents it knows and how to connect one", () => {
		const html = draw({ can_write: true });
		expect(html).toContain("No agent connected");
		expect(html).toContain("OpenCode");
		expect(html).toContain("Found on this machine");
		expect(html).toContain("never installs an agent");
		expect(html).toContain('data-testid="chat-profile-form"');
	});

	test("shows the full command of a profile, as text", () => {
		const html = draw({
			can_write: true,
			profiles: [
				{
					name: "mine",
					command: "C:\\Program Files\\Agent\\agent.exe",
					args: ["--mode", "<b>x</b>"],
					created_by: "local:ui",
					created_at: "2026-11-19T12:00:00Z",
					updated_at: "2026-11-19T12:00:00Z",
				},
			],
		});
		expect(html).toContain("Program Files");
		expect(html).toContain("&lt;b&gt;x&lt;/b&gt;");
		expect(html).not.toContain("<b>x</b>");
	});

	test("a person who may not write a profile is sent to the administrator", () => {
		const html = draw({ can_write: false }, true);
		expect(html).toContain("Ask the administrator");
		expect(html).not.toContain('data-testid="chat-profile-form"');
	});

	test("a session that is not a person's is told where to do it, not sent to an administrator (VV-331)", () => {
		const html = draw({ can_write: false }, false);
		expect(html).toContain("Only a person in the Stuarty desktop app");
		expect(html).not.toContain("Ask the administrator");
		expect(html).not.toContain('data-testid="chat-profile-form"');
	});

	test("says when this process cannot run an agent", () => {
		expect(draw({ available: false })).toContain("cannot run an agent");
	});
});
