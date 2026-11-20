import { Link } from "@inertiajs/react";
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

	test("a child with an href is announced as a link", () => {
		expect(
			renderToString(
				<Button asChild>
					<a href="/projects">Open</a>
				</Button>,
			),
		).toMatch(/^<a [^>]*role="link"/);
		expect(
			renderToString(
				<Button asChild>
					<Link href="/projects">Open</Link>
				</Button>,
			),
		).toMatch(/^<a [^>]*role="link"/);
	});

	test("any other child keeps the button role", () => {
		expect(
			renderToString(
				<Button asChild>
					<span>Snap</span>
				</Button>,
			),
		).toMatch(/^<span [^>]*role="button"/);
	});
});
