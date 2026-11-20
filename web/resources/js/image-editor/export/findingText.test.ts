import { describe, expect, test } from "vitest";

import { withLayerName } from "./findingText";

describe("the layer a finding names", () => {
	const message = "The text of layer lyr_0000000001 is 8 px.";

	test("the name of the layer replaces its id", () => {
		expect(withLayerName(message, "lyr_0000000001", "Title")).toBe(
			"The text of layer “Title” is 8 px.",
		);
	});

	test("a layer with no name keeps the id", () => {
		expect(withLayerName(message, "lyr_0000000001", "")).toBe(message);
		expect(withLayerName(message, "lyr_0000000001", undefined)).toBe(message);
		expect(withLayerName(message, undefined, "Title")).toBe(message);
	});
});
