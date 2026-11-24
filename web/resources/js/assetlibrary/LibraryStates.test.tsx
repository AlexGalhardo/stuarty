import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import {
	LibraryEmpty,
	LibraryLoading,
	LibraryReadFailed,
} from "./LibraryStates";

vi.mock("@inertiajs/react", () => ({
	Link: ({ children, href }: { children: string; href: string }) => (
		<a href={href}>{children}</a>
	),
}));

const messages = {
	"ui.assets.empty_title.video": "No videos yet",
	"ui.assets.empty.video": "Nothing here yet.",
	"ui.assets.loading.video": "Loading the videos...",
	"ui.assets.guide.title": "How a library works",
	"ui.assets.guide.bring.title": "Bring a file in",
	"ui.assets.guide.bring.text": "Import it.",
	"ui.assets.guide.tag.title": "Tag it",
	"ui.assets.guide.tag.text": "Tags.",
	"ui.assets.guide.use.title": "Use it",
	"ui.assets.guide.use.text": "No copy.",
	"ui.assets.guide.open_projects": "Open the projects",
	"ui.assets.error.retry": "Try again",
	"ui.assets.error.how.video": "How to fix",
	"ui.assets.error.step1.title": "Try again",
	"ui.assets.error.step1.text": "Nothing changed.",
	"ui.assets.error.step2.title": "Restart",
	"ui.assets.error.step2.text": "Files are kept.",
};

function render(node: React.ReactNode): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			{node}
		</I18nProvider>,
	);
}

describe("the states of a library", () => {
	test("an empty library says what it holds, gives three steps and a way on, and assumes no media", () => {
		const html = render(<LibraryEmpty section="video" />);
		expect(html).toContain("No videos yet");
		expect(html).toContain("<ol");
		expect(html).toContain('href="/projects"');
	});

	test("loading is a polite status sentence over a hidden skeleton, with no spinner", () => {
		const html = render(<LibraryLoading section="video" />);
		expect(html).toContain('role="status"');
		expect(html).toContain("Loading the videos...");
		expect(html).toContain('aria-hidden="true"');
		expect(html).not.toContain("animate-spin");
	});

	test("a failed read offers a retry and the two steps", () => {
		const html = render(
			<LibraryReadFailed section="video" onRetry={() => undefined} />,
		);
		expect(html).toContain("Try again");
		expect(html).toContain("Restart");
	});
});
