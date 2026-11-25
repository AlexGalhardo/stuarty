import { expect, test } from "vitest";

import { optionKey, questionKey, savedAnswers } from "./Briefing";

test("savedAnswers maps the answers a brief holds by question id", () => {
	expect(
		savedAnswers({
			production_kind: "short",
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
		savedAnswers({
			production_kind: "short",
			questions: [],
			has_brief: false,
			guided: false,
			text: "",
		}),
	).toEqual({});
});

// VV-399: a text and an image say "for whom" in words of their own, the video sets keep theirs.
test("the words of a question follow the set the service served", () => {
	expect(questionKey("text", "audience")).toBe(
		"ui.projects.briefing.text.q.audience",
	);
	expect(questionKey("image", "audience")).toBe(
		"ui.projects.briefing.image.q.audience",
	);
	for (const set of ["short", "commercial", "long"]) {
		expect(questionKey(set, "audience")).toBe(
			"ui.projects.briefing.q.audience",
		);
	}
	expect(optionKey("image", "channel", "blog")).toBe(
		"ui.projects.briefing.image.option.channel.blog",
	);
	expect(optionKey("text", "channel", "blog")).toBe(
		"ui.projects.briefing.text.option.channel.blog",
	);
	expect(optionKey("short", "platform", "tiktok")).toBe(
		"ui.projects.briefing.option.platform.tiktok",
	);
});
