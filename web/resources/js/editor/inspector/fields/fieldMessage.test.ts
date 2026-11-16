import { expect, test } from "vitest";

import { rangeOf, withoutArgPath } from "./fieldMessage";

test("the argument path of a core message is dropped", () => {
	expect(withoutArgPath("args.crop: must be below 1")).toBe("must be below 1");
	expect(withoutArgPath("args.crop.left: too wide")).toBe("too wide");
	expect(withoutArgPath("an ordinary message")).toBe("an ordinary message");
});

test("the limits of a refused number are read from the details", () => {
	expect(rangeOf({ min: 0, max: 1 })).toEqual({ min: 0, max: 1 });
	expect(rangeOf({ min: 0 })).toBeNull();
	expect(rangeOf(undefined)).toBeNull();
});
