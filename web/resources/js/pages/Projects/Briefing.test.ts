import { expect, test } from "vitest";

import { savedAnswers } from "./Briefing";

test("savedAnswers maps the answers a brief holds by question id", () => {
	expect(
		savedAnswers({
			questions: [],
			has_brief: true,
			guided: true,
			text: "",
			answers: [
				{ id: "audience", value: "makers" },
				{ id: "duration", value: "30" },
			],
		}),
	).toEqual({ audience: "makers", duration: "30" });
	expect(
		savedAnswers({ questions: [], has_brief: false, guided: false, text: "" }),
	).toEqual({});
});
