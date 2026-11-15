import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { ExportCapabilities, ExportOptions } from "../api/types";
import { devFixture } from "../boot/devFixture";
import ExportOptionsFields, { type Unsupported } from "./ExportOptions";

function render(
	capabilities: ExportCapabilities,
	value: ExportOptions = {},
	unsupported: Unsupported | null = null,
): string {
	return renderToString(
		<I18nProvider messages={devFixture(1).bootstrap.messages} locale="en">
			<ExportOptionsFields
				capabilities={capabilities}
				value={value}
				disabled={false}
				unsupported={unsupported}
				onChange={() => undefined}
			/>
		</I18nProvider>,
	);
}

describe("ExportOptionsFields", () => {
	test("a server that exposes no option shows none", () => {
		expect(render({ formats: [], fps: [], sizes: [] })).toBe("");
	});

	test("only the members the server lists are offered", () => {
		const html = render({ formats: ["mp4", "gif"], fps: [60], sizes: [] });
		expect(html).toContain("GIF animation");
		expect(html).not.toContain("WAV audio only");
		expect(html).toContain("60 fps");
		expect(html).not.toContain('id="export-size"');
	});

	test("audio only turns the rate and the size off", () => {
		const html = render(
			{ formats: ["wav"], fps: [60], sizes: ["3840x2160"] },
			{ format: "wav" },
		);
		expect(html.match(/disabled=""/g)?.length).toBe(2);
	});

	test("the core refusal is shown with its code and hint", () => {
		const html = render(
			{ formats: [], fps: [60], sizes: ["3840x2160"] },
			{ size: "3840x2160" },
			{ code: "E_NO_ENCODER", message: "no encoder", hint: "lower the size" },
		);
		expect(html).toContain("E_NO_ENCODER");
		expect(html).toContain("lower the size");
	});
});
