import { Film } from "lucide-react";
import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import Section, { ToolLine } from "./Section";

describe("inspector Section", () => {
	test("is an open region with its header as a button that controls the body", () => {
		const html = renderToString(
			<Section title="Transform" name="transform">
				<p>fields</p>
			</Section>,
		);
		expect(html).toContain('data-section="transform"');
		expect(html).toContain('aria-expanded="true"');
		expect(html).toContain("aria-controls=");
		expect(html).not.toContain('hidden=""');
	});

	test("starts closed, with the body hidden but in the document, when it asks to", () => {
		const html = renderToString(
			<Section title="Transform" defaultOpen={false}>
				<p>fields</p>
			</Section>,
		);
		expect(html).toContain('aria-expanded="false"');
		expect(html).toContain('hidden=""');
		expect(html).toContain("fields");
	});

	test("draws its icon before the title and the chevron after it", () => {
		const html = renderToString(
			<Section title="Transform" icon={Film}>
				<p>fields</p>
			</Section>,
		);
		expect(html.indexOf("lucide-film")).toBeGreaterThan(-1);
		expect(html.indexOf("lucide-film")).toBeLessThan(html.indexOf("Transform"));
		expect(html.indexOf("Transform")).toBeLessThan(
			html.indexOf("lucide-chevron-up"),
		);
	});

	test("a tool line names the command of the registry next to the word Tool", () => {
		const html = renderToString(
			<I18nProvider
				locale="pt-BR"
				messages={{ "ui.editor.tool": "Ferramenta" }}
			>
				<ToolLine name="clip.blur_region" />
			</I18nProvider>,
		);
		expect(html).toContain("Ferramenta");
		expect(html).toContain('data-tool="clip.blur_region"');
		expect(html).toContain(">clip.blur_region<");
	});
});
