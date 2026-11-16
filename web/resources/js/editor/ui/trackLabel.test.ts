import { expect, test } from "vitest";

import { trackLabel } from "./trackLabel";

test("tracks are numbered from the bottom, per kind", () => {
	const order = ["t_a2", "t_v2", "t_a1", "t_v1"];
	const kindOf = (id: string): "audio" | "visual" =>
		id.startsWith("t_a") ? "audio" : "visual";
	expect(order.map((id) => trackLabel(id, order, kindOf))).toEqual([
		"A2",
		"V2",
		"A1",
		"V1",
	]);
});
