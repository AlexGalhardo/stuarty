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
		expect(render({ formats: [] })).toBe("");
	});

	test("only the formats the server lists are offered", () => {
		const html = render({ formats: ["mp4", "gif"] });
		expect(html).toContain("GIF animation");
		expect(html).not.toContain("WAV audio only");
		expect(html).not.toContain("export-fps");
		expect(html).not.toContain("export-size");
	});

	test("the core refusal is shown with its code and hint", () => {
		const html = render(
			{ formats: ["mp4", "gif"] },
			{ format: "gif" },
			{
				code: "E_NO_ENCODER",
				message: "no encoder",
				hint: "use another format",
			},
		);
		expect(html).toContain("E_NO_ENCODER");
		expect(html).toContain("use another format");
	});
});
