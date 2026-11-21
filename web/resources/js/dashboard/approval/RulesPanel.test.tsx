import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { type SessionInfo, SessionProvider } from "@/lib/session";
import type { ApprovalRule } from "../../editor/api/types";
import RulesPanel from "./RulesPanel";

const rule = {
	id: "rul_1",
	kind: "publish",
	connection_id: null,
	project_id: null,
	enabled: true,
} as unknown as ApprovalRule;

function markup(session: SessionInfo): string {
	return renderToString(
		<I18nProvider messages={{}} locale="en" locales={["en"]}>
			<SessionProvider session={session}>
				<RulesPanel
					rules={[rule]}
					destinations={{}}
					onChanged={() => undefined}
				/>
			</SessionProvider>
		</I18nProvider>,
	);
}

// The attribute, not the Tailwind variants (`disabled:opacity-50`) in the class.
const DISABLED = /\sdisabled(=|\s|>)/;

// VV-319: turning a rule off and removing it need a person as writing one does (ApprovalRuleSet and
// ApprovalRuleRemove are refused with E_PRIVILEGED_SESSION), so in a session that is not a person's they are
// disabled and point at the reason.
describe("the buttons of a rule", () => {
	const ruleButtons = (html: string): string[] => {
		const row = html.slice(html.indexOf('data-rule-id="rul_1"'));
		return (row.match(/<button[^>]*>/g) ?? []).slice(0, 2);
	};

	test("are disabled, with the reason, in a session that is not a person's", () => {
		const found = ruleButtons(markup({ privileged: false, mode: "local" }));
		expect(found).toHaveLength(2);
		for (const button of found) {
			expect(button).toMatch(DISABLED);
			expect(button).toContain('aria-describedby="rules-person-only"');
		}
	});

	test("are enabled in a person's session", () => {
		const found = ruleButtons(markup({ privileged: true, mode: "local" }));
		expect(found).toHaveLength(2);
		for (const button of found) expect(button).not.toMatch(DISABLED);
	});
});
