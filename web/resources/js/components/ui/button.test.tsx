import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { Button } from "./button";

describe("Button", () => {
	test.each([
		"ghost",
		"secondary",
		"outline",
	] as const)("a pressed %s toggle is lit by its aria-pressed state", (variant) => {
		const html = renderToString(
			<Button variant={variant} aria-pressed={true}>
				Snap
			</Button>,
		);
		expect(html).toContain('aria-pressed="true"');
		expect(html).toContain("aria-pressed:bg-accent");
		expect(html).toContain("aria-pressed:border-foreground/50");
	});
});
