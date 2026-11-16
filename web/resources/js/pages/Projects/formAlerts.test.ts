import { expect, test } from "vitest";

import { clearOtherAlerts, onOtherSubmit } from "./formAlerts";

test("submitting one form clears the alerts of the others only", () => {
	const cleared: string[] = [];
	const offA = onOtherSubmit("a", () => cleared.push("a"));
	const offB = onOtherSubmit("b", () => cleared.push("b"));
	clearOtherAlerts("a");
	expect(cleared).toEqual(["b"]);
	offA();
	offB();
	clearOtherAlerts("a");
	expect(cleared).toEqual(["b"]);
});
