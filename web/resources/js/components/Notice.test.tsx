import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { Notice } from "./Notice";

describe("Notice", () => {
	test("tells its tone with an icon and a title, not by colour alone", () => {
		for (const tone of ["info", "success", "warning", "danger"] as const) {
			const html = renderToString(
				<Notice tone={tone} title="Heads up">
					Body
				</Notice>,
			);
			expect(html).toContain(`data-tone="${tone}"`);
			expect(html).toContain("<svg");
			expect(html).toContain('aria-hidden="true"');
			expect(html).toContain("Heads up");
			expect(html).toContain("Body");
		}
	});

	test("is a note unless the caller says it appears", () => {
		expect(renderToString(<Notice tone="info" title="a" />)).toContain(
			'role="note"',
		);
		expect(
			renderToString(<Notice tone="danger" title="a" role="alert" />),
		).toContain('role="alert"');
	});
});
