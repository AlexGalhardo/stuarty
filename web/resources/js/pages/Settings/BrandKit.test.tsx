import { describe, expect, test, vi } from "vitest";

import { logoLabel } from "./BrandKit";

vi.mock("@inertiajs/react", () => ({ Head: () => null }));

const asset = {
	hash: `sha256:${"a".repeat(64)}`,
	name: "logo.png",
	kind: "image",
	bytes: 10,
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
