import { describe, expect, test, vi } from "vitest";
import type { Transport, TransportRequest } from "@/lib/api";
import { drawForTest } from "@/workflow/drawForTest";
import { createClient } from "../../editor/api/client";
import Index from "./Index";

vi.mock("@inertiajs/react", () => ({
	Head: ({ title }: { title: string }) => <title>{title}</title>,
}));

describe("EditVideo/Index", () => {
	test("renders dropzone, file input, and 4 aspect-ratio presets in English and pt-BR", () => {
		const htmlEn = drawForTest(<Index />, "en");
		expect(htmlEn).toContain('data-testid="edit-video-dropzone"');
		expect(htmlEn).toContain('data-testid="edit-video-file-input"');
		expect(htmlEn).toContain('data-testid="edit-video-choose-file"');
		expect(htmlEn).toContain('data-testid="edit-video-preset-9:16"');
		expect(htmlEn).toContain('data-testid="edit-video-preset-16:9"');
		expect(htmlEn).toContain('data-testid="edit-video-preset-1:1"');
		expect(htmlEn).toContain('data-testid="edit-video-preset-4:5"');
		expect(htmlEn).toContain("Edit video");

		const htmlPt = drawForTest(<Index />, "pt-BR");
		expect(htmlPt).toContain("Editar vídeo");
		expect(htmlPt).toContain('data-testid="edit-video-dropzone"');
	});

	test("standalone client methods hit the expected API routes", async () => {
		const calls: TransportRequest[] = [];
		const transport: Transport = {
			openStream() {
				throw new Error("unused");
			},
			async send(req) {
				calls.push(req);
				if (req.path === "/standalone/videos" && req.method === "GET") {
					return {
						status: 200,
						body: JSON.stringify({ projects: [], next_cursor: null }),
					};
				}
				return {
					status: 200,
					body: JSON.stringify({
						project_id: "prj_stand00001",
						timeline_id: "prj_stand00001",
						name: "Clip",
						width: 1920,
						height: 1080,
						fps: "30",
						standalone: true,
					}),
				};
			},
		};
		const client = createClient(transport);

		await client.listStandaloneVideos();
		await client.createStandaloneVideo({ preset: "16:9" });
		await client.promoteStandaloneVideo("prj_stand00001", {
			name: "My Launch Video",
			description: "Full production description",
			goal: {
				specific: "Publish video",
				measurable: "1000 views",
				achievable: "Already edited",
				relevant: "Launch week",
				time_bound: "This Friday",
				due: "2026-12-31",
			},
			kind: "video",
		});

		expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
			"GET /standalone/videos",
			"POST /standalone/videos",
			"POST /standalone/videos/prj_stand00001/promote",
		]);
	});
});
