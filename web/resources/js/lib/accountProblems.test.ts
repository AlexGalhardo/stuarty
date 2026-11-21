import { describe, expect, test } from "vitest";

import { ApiError } from "../editor/api/errors";
import { fieldProblems, reasonsOf } from "./accountProblems";

describe("fieldProblems", () => {
	test("reads the fields of a validation error", () => {
		const error = new ApiError(400, {
			code: "E_VALIDATION",
			message: "bad",
			details: {
				fields: [
					{ field: "username", reason: "too_short" },
					{ field: "password", reason: "no_digit" },
					{ field: "password", reason: "no_special" },
				],
			},
		});
		const problems = fieldProblems(error);
		expect(problems).toHaveLength(3);
		expect(reasonsOf(problems, "password")).toEqual(["no_digit", "no_special"]);
		expect(reasonsOf(problems, "email")).toEqual([]);
	});

	test("reads nothing from another error or a shape it does not know", () => {
		expect(fieldProblems(new Error("x"))).toEqual([]);
		expect(
			fieldProblems(
				new ApiError(401, { code: "E_UNAUTHORIZED", message: "no" }),
			),
		).toEqual([]);
		expect(
			fieldProblems(
				new ApiError(400, {
					code: "E_VALIDATION",
					message: "bad",
					details: { fields: [{ field: 3 }, "x", null, []] },
				}),
			),
		).toEqual([]);
	});
});
