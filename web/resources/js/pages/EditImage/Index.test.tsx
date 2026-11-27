import { describe, expect, test, vi } from "vitest";

import type { Transport, TransportRequest } from "@/lib/api";
import { drawForTest } from "@/workflow/drawForTest";
import { createImageApi } from "../../image-editor/api/imageApi";
import Index from "./Index";

vi.mock("@inertiajs/react", () => ({
	Head: ({ title }: { title: string }) => <title>{title}</title>,
	Link: ({
		children,
		href,
		className,
	}: {
		children: unknown;
		href: string;
		className?: string;
	}) => (
		<a href={href} className={className}>
			{children as string}
		</a>
	),
}));

describe("EditImage/Index", () => {
	test("renders instant dropzone, file input, and existing new-image controls in EN and pt-BR", () => {
		const htmlEn = drawForTest(<Index />, "en");
		expect(htmlEn).toContain('data-testid="edit-image-dropzone"');
		expect(htmlEn).toContain('data-testid="edit-image-file-input"');
		expect(htmlEn).toContain('data-testid="edit-image-choose-file"');
		expect(htmlEn).toContain('data-testid="image-new-form"');
		expect(htmlEn).toContain('data-testid="image-import-open"');
		expect(htmlEn).toContain("Drop an image file here");

		const htmlPt = drawForTest(<Index />, "pt-BR");
		expect(htmlPt).toContain("Solte um arquivo de imagem aqui");
		expect(htmlPt).toContain('data-testid="edit-image-dropzone"');
	});

	test("image API standalone helpers call the expected routes", async () => {
		const calls: TransportRequest[] = [];
		const transport: Transport = {
			openStream() {
				throw new Error("unused");
			},
			async send(req) {
				calls.push(req);
				if (req.path === "/projects") {
					return { status: 200, body: JSON.stringify({ projects: [] }) };
				}
				return {
					status: 200,
					body: JSON.stringify({
						state: "ready",
						image_id: "img_00000001",
						text: "Opened",
					}),
				};
			},
		};
		const api = createImageApi(transport);

		await api.openFromAsset(`sha256:${"b".repeat(64)}`);
		await api.listProjects();
		await api.attachExportToProject("prj_alpha00001", "/tmp/out.png");

		expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
			"POST /images/from-asset",
			"GET /projects",
			"POST /projects/prj_alpha00001/media",
		]);
	});
});
