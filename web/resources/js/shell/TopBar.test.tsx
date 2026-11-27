import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";
import TopBar from "./TopBar";
import { Frame } from "./testSupport";

function draw(url: string, drawer = false, bell?: string): string {
	return renderToString(
		<Frame>
			<TopBar
				url={url}
				drawer={drawer}
				menuOpen={false}
				onOpenMenu={() => undefined}
			>
				{bell === undefined ? null : <span data-testid="slot">{bell}</span>}
			</TopBar>
		</Frame>,
	);
}

describe("TopBar", () => {
	test("is the banner of the page, with the breadcrumb and the Chat button", () => {
		const html = draw("/publish");
		expect(html).toMatch(/^<header [^>]*data-testid="topbar"/);
		expect(html).toContain('aria-label="Breadcrumb"');
		expect(html).toContain('data-testid="chat-toggle"');
		expect(html).toContain('aria-label="Chat with your agent"');
		expect(html).toContain('aria-controls="chat-panel"');
	});

	test("the last step is the current page and only it", () => {
		const html = draw("/publish");
		expect(html.match(/aria-current="page"/g)).toHaveLength(1);
		expect(html).toMatch(/aria-current="page"[^>]*>Publishing</);
		expect(html).toMatch(/href="\/approvals">Publish</);
	});

	test("shows the menu button only when the sidebar is a drawer", () => {
		expect(draw("/", true)).toContain('data-testid="menu-toggle"');
		expect(draw("/", false)).not.toContain('data-testid="menu-toggle"');
		expect(draw("/", true)).toContain('aria-label="Open the navigation"');
	});

	test("holds what the layout puts between the trail and the Chat button", () => {
		const html = draw("/", false, "bell");
		expect(html.indexOf("breadcrumb")).toBeLessThan(
			html.indexOf('data-testid="slot"'),
		);
		expect(html.indexOf('data-testid="slot"')).toBeLessThan(
			html.indexOf('data-testid="chat-toggle"'),
		);
	});

	test("renders the Back button disabled on Home with no history and enabled on subpages", () => {
		const homeHtml = draw("/");
		expect(homeHtml).toContain('data-testid="topbar-back"');
		expect(homeHtml).toMatch(/disabled=""[^>]*data-testid="topbar-back"/);

		const subHtml = draw("/publish");
		expect(subHtml).toContain('data-testid="topbar-back"');
		expect(subHtml).not.toMatch(/disabled=""[^>]*data-testid="topbar-back"/);
	});
});
