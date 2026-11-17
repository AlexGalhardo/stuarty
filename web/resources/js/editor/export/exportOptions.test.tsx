import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { ExportCapabilities, ExportOptions } from "../api/types";
import { devFixture } from "../boot/devFixture";
import ExportOptionsFields, {
	formatOptions,
	rangeOptions,
	type Unsupported,
} from "./ExportOptions";

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
		expect(render({ formats: [], range: false })).toBe("");
	});

	test("only the formats the server lists are offered", () => {
		const html = render({ formats: ["mp4", "gif"], range: false });
		expect(html).toContain("GIF animation");
		expect(html).not.toContain("WAV audio only");
		expect(html).not.toContain("export-fps");
		expect(html).not.toContain("export-size");
	});

	test("the core refusal is shown with its code and hint", () => {
		const html = render(
			{ formats: ["mp4", "gif"], range: false },
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
	test("the range checkbox needs a point and a server that takes start and end", () => {
		const caps = { formats: ["mp4"], range: true };
		const html = renderToString(
			<I18nProvider messages={devFixture(1).bootstrap.messages} locale="en">
				<ExportOptionsFields
					capabilities={caps}
					value={{}}
					disabled={false}
					unsupported={null}
					range={{ in: 30, out: null }}
					onChange={() => undefined}
				/>
			</I18nProvider>,
		);
		expect(html).toContain("export-range");
		expect(render(caps)).not.toContain("export-range");
		expect(render({ formats: ["mp4"], range: false })).not.toContain(
			"export-range",
		);
	});

	test("the range goes as the points that are set, with a unit", () => {
		expect(rangeOptions({ format: "gif" }, { in: 30, out: 90 }, true)).toEqual({
			format: "gif",
			start: "30f",
			end: "90f",
		});
		expect(rangeOptions({}, { in: null, out: 90 }, true)).toEqual({
			end: "90f",
		});
		expect(
			rangeOptions(
				{ format: "gif", start: "30f", end: "90f" },
				{ in: 30, out: 90 },
				false,
			),
		).toEqual({ format: "gif" });
	});
});

describe("formatOptions", () => {
	test("png is one frame at the playhead and drops the range", () => {
		expect(formatOptions({ start: "30f", end: "90f" }, "png", 45)).toEqual({
			format: "png",
			at: "45f",
		});
	});

	test("another format drops the frame and keeps the range; mp4 is the default", () => {
		expect(formatOptions({ format: "png", at: "45f" }, "gif", 10)).toEqual({
			format: "gif",
			at: undefined,
		});
		expect(
			formatOptions({ format: "png", at: "45f", start: "1f" }, "mp4", 10),
		).toEqual({ format: undefined, at: undefined, start: "1f" });
	});

	test("the png choice is labelled and hides the range", () => {
		const caps = { formats: ["mp4", "png"], range: true };
		const html = renderToString(
			<I18nProvider messages={devFixture(1).bootstrap.messages} locale="en">
				<ExportOptionsFields
					capabilities={caps}
					value={{ format: "png", at: "5f" }}
					disabled={false}
					unsupported={null}
					range={{ in: 30, out: 90 }}
					onChange={() => undefined}
				/>
			</I18nProvider>,
		);
		expect(html).toContain("PNG image of the frame at the playhead");
		expect(html).not.toContain("export-range");
	});
});
