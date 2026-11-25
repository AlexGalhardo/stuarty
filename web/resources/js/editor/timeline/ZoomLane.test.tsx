import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { devFixture } from "../boot/devFixture";
import { zoomClearOperations } from "../commands/builders";
import { ZoomLane } from "./ZoomLane";

const messages = devFixture(1).bootstrap.messages;
const time = (frame: number) => ({ frame, seconds: frame / 30 });

describe("the zoom lane (final design 25)", () => {
	test("draws one numbered box for each zoom the service read, at its frames", () => {
		const html = renderToString(
			<I18nProvider locale="en" messages={messages}>
				<ZoomLane
					ppf={2}
					zooms={[
						{ item_id: "itm_a", n: 1, start: time(30), end: time(120) },
						{ item_id: "itm_a", n: 2, start: time(300), end: time(360) },
					]}
				/>
			</I18nProvider>,
		);
		expect(html).toContain('aria-label="Zoom lane Z1, 2 zooms"');
		expect(html).toContain('data-zoom="1"');
		expect(html).toContain('data-zoom="2"');
		expect(html).toContain("left:60px;width:180px");
	});

	test("names one zoom in the singular", () => {
		const html = renderToString(
			<I18nProvider locale="en" messages={messages}>
				<ZoomLane
					ppf={2}
					zooms={[{ item_id: "itm_a", n: 1, start: time(0), end: time(30) }]}
				/>
			</I18nProvider>,
		);
		expect(html).toContain('aria-label="Zoom lane Z1, 1 zoom"');
	});
});

describe("the zoom switch off", () => {
	test("clears the position and the scale of the clip, and nothing else", () => {
		expect(zoomClearOperations("itm_a")).toEqual([
			{ op: "keyframe.clear", args: { item_id: "itm_a", property: "x" } },
			{ op: "keyframe.clear", args: { item_id: "itm_a", property: "y" } },
			{ op: "keyframe.clear", args: { item_id: "itm_a", property: "scale" } },
		]);
	});
});
