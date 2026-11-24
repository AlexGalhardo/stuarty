import { describe, expect, it } from "vitest";

import type { CutsProposal, ToolsStatusResult } from "../../api/types";
import {
	clockLabel,
	faceOffer,
	isFaceUnavailable,
	sharePercent,
	timelineTitle,
} from "./verticalCutsModel";

const status = (offers: ToolsStatusResult["offers"]): ToolsStatusResult => ({
	installed: [],
	offers,
	state: "idle",
	done_bytes: 0,
	total_bytes: 0,
	text: "",
});

describe("verticalCutsModel", () => {
	it("finds the face offer and nothing else", () => {
		const offer = {
			name: "face",
			files: [],
			download_bytes: 5,
			published: true,
			installed: false,
		};
		expect(faceOffer(status([offer]))).toBe(offer);
		expect(faceOffer(status([{ ...offer, name: "other" }]))).toBeUndefined();
		expect(faceOffer(status([]))).toBeUndefined();
	});

	it("tells the missing face set from the missing transcription helper", () => {
		expect(
			isFaceUnavailable({
				code: "E_TRANSCRIBE_UNAVAILABLE",
				details: { tool: "face" },
			}),
		).toBe(true);
		expect(
			isFaceUnavailable({
				code: "E_TRANSCRIBE_UNAVAILABLE",
				details: { tool: "whisper" },
			}),
		).toBe(false);
		expect(
			isFaceUnavailable({ code: "E_VALIDATION", details: { tool: "face" } }),
		).toBe(false);
		expect(isFaceUnavailable({ code: "E_TRANSCRIBE_UNAVAILABLE" })).toBe(false);
	});

	it("labels times and shares", () => {
		expect(clockLabel(0)).toBe("0:00");
		expect(clockLabel(65_900)).toBe("1:05");
		expect(clockLabel(600_000)).toBe("10:00");
		expect(sharePercent(1000)).toBe(100);
		expect(sharePercent(727)).toBe(73);
	});

	it("builds a title an item accepts", () => {
		const proposal = (title: string): CutsProposal => ({
			n: 2,
			start_ms: 0,
			end_ms: 1,
			words: 1,
			face_share: 900,
			score: 1,
			title,
			batch: [],
		});
		expect(timelineTitle(proposal("hello there"))).toBe("Cut 2: hello there");
		const long = timelineTitle(proposal("x".repeat(500)));
		expect(long).toHaveLength(120);
		expect(long.endsWith("…")).toBe(true);
	});
});
