import { describe, expect, test, vi } from "vitest";

import { drawForTest } from "../../workflow/drawForTest";
import BrandKitPage, { logoLabel } from "./BrandKit";

vi.mock("@inertiajs/react", () => ({ Head: () => null }));

const asset = {
	hash: `sha256:${"a".repeat(64)}`,
	name: "logo.png",
	kind: "image",
	bytes: 10,
	size_text: "10 B",
	state: "ready" as const,
	projects: 1,
};

describe("the logo picker label", () => {
	test("names the file and its size when the service knows it", () => {
		expect(logoLabel({ ...asset, width: 512, height: 256 })).toBe(
			"logo.png (512x256)",
		);
	});

	test("is the bare name when the size is unknown", () => {
		expect(logoLabel(asset)).toBe("logo.png");
	});
});

describe("the brand kit page on the shared components", () => {
	test("opens with the page header and a loading state while the kit is read", () => {
		const html = drawForTest(<BrandKitPage />);
		expect(html).toMatch(/<h1[^>]*>Brand kit<\/h1>/);
		expect(html).toContain('data-slot="page-header"');
		expect(html).toContain("A logo, named colours and fonts");
		expect(html).toContain('data-slot="loading-state"');
		expect(html).not.toContain("ui.brand");
	});

	test("says it in Portuguese", () => {
		const html = drawForTest(<BrandKitPage />, "pt-BR");
		expect(html).toMatch(/<h1[^>]*>Kit de marca<\/h1>/);
		expect(html).not.toContain("ui.brand");
	});
});
