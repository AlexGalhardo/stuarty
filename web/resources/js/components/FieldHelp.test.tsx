import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import { I18nProvider } from "@/lib/i18n";
import { FieldHelpTip, FieldHint, FieldLabel, HelpCard } from "./FieldHelp";

const MESSAGES = {
	"ui.help.more_info": "More information about {field}",
	"ui.help.field": "this field",
	"ui.help.examples": "Examples:",
};

function render(node: React.ReactNode): string {
	return renderToString(
		<I18nProvider locale="en" messages={MESSAGES}>
			<TooltipProvider>{node}</TooltipProvider>
		</I18nProvider>,
	);
}

describe("FieldHelp primitives (epic 112)", () => {
	test("FieldLabel pairs a label with a keyboard-focusable tooltip button", () => {
		const html = render(
			<FieldLabel
				htmlFor="api-key-name"
				label="Key name"
				help="A short name that helps you recognize this key later."
			/>,
		);
		expect(html).toContain('for="api-key-name"');
		expect(html).toContain("Key name");
		expect(html).toContain('type="button"');
		expect(html).toContain('aria-label="More information about Key name"');
		expect(html).toContain('data-slot="field-help-trigger"');
		expect(html).toContain('aria-hidden="true"');
	});

	test("FieldHelpTip works with custom aria-label and compact 24px target", () => {
		const html = render(
			<FieldHelpTip
				ariaLabel="Explain bitrate"
				help="Controls output video quality."
			/>,
		);
		expect(html).toContain('aria-label="Explain bitrate"');
		expect(html).toContain("size-6");
	});

	test("FieldHint renders an id for aria-describedby linkage", () => {
		const html = render(
			<FieldHint id="webhook-url-hint">
				Only HTTPS addresses are accepted.
			</FieldHint>,
		);
		expect(html).toContain('id="webhook-url-hint"');
		expect(html).toContain('data-slot="field-hint"');
		expect(html).toContain("Only HTTPS addresses are accepted.");
	});

	test("HelpCard renders a note with icon, title, description, and examples", () => {
		const html = render(
			<HelpCard
				title="Prompt variables"
				description="Wrap placeholders in double braces to fill them when you run the prompt."
				examples={["{{topic}}", "{{audience}}"]}
			/>,
		);
		expect(html).toContain('role="note"');
		expect(html).toContain('data-slot="help-card"');
		expect(html).toContain("Prompt variables");
		expect(html).toContain("Examples:");
		expect(html).toContain("{{topic}}");
		expect(html).toContain("{{audience}}");
	});

	test("HelpCard supports collapsible details mode", () => {
		const html = render(
			<HelpCard
				collapsible
				title="How webhooks work"
				description="Stuarty sends signed POST requests with event IDs."
			/>,
		);
		expect(html).toContain("<details");
		expect(html).toContain("<summary");
		expect(html).toContain("How webhooks work");
	});
});
