import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import ErrorPage from "./Error";

vi.mock("@inertiajs/react", () => ({
	Head: () => null,
	Link: ({ children, href }: { children: unknown; href: string }) => (
		<a href={href}>{children as string}</a>
	),
}));

const messages = {
	"ui.error.back": "Back to projects",
	"ui.error.home": "Go to the home",
	"ui.error.label": "Error {status}",
	"ui.error.how_to_fix": "How to fix:",
	"ui.error.next.not_found.title": "Next, not found",
	"ui.error.next.forbidden.title": "Next, forbidden",
	"ui.error.next.server.title": "Next, server",
	"ui.error.next.other.title": "Next, other",
	"ui.error.next.not_found.body": "b",
	"ui.error.next.forbidden.body": "b",
	"ui.error.next.server.body": "b",
	"ui.error.next.other.body": "b",
};

function page(props: Parameters<typeof ErrorPage>[0]): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<ErrorPage {...props} />
		</I18nProvider>,
	);
}

describe("the error page", () => {
	test("shows the text of the core as it came, as text", () => {
		const html = page({
			status: 422,
			message: "Items <b>would</b> overlap on track trk_1.",
			code: "E_OVERLAP",
			hint: "move the item & try again",
		});
		expect(html).toContain(
			"Items &lt;b&gt;would&lt;/b&gt; overlap on track trk_1.",
		);
		expect(html).toContain("move the item &amp; try again");
		expect(html).toContain("E_OVERLAP");
		expect(html).not.toContain("<b>");
		expect(html.match(/<h1/g)).toHaveLength(1);
	});

	test("says what to do next, by the kind of error", () => {
		expect(page({ status: 404, message: "Not Found" })).toContain(
			"Next, not found",
		);
		expect(page({ status: 403, message: "Forbidden" })).toContain(
			"Next, forbidden",
		);
		expect(page({ status: 500, message: "Internal Server Error" })).toContain(
			"Next, server",
		);
		expect(page({ status: 422, message: "x" })).toContain("Next, other");
	});

	test("offers the way back with a link", () => {
		const html = page({ status: 404, message: "Not Found" });
		expect(html).toContain('href="/projects"');
		expect(html).toContain('href="/"');
		expect(html).toContain("Error 404");
	});

	test("shows no hint or code that the server did not send", () => {
		const html = page({ status: 500, message: "Internal Server Error" });
		expect(html).not.toContain("How to fix:");
		expect(html).not.toContain("font-mono");
	});
});
