import { describe, expect, test, vi } from "vitest";

import { drawForTest } from "../workflow/drawForTest";
import ErrorPage from "./Error";

vi.mock("@inertiajs/react", () => ({
	Head: () => null,
	Link: ({ children, href }: { children: unknown; href: string }) => (
		<a href={href}>{children as string}</a>
	),
}));

function page(
	props: Parameters<typeof ErrorPage>[0],
	locale: "en" | "pt-BR" = "pt-BR",
): string {
	return drawForTest(<ErrorPage {...props} />, locale);
}

describe("the error page", () => {
	test("the title is in the language of the person, never the server message", () => {
		const html = page({
			status: 404,
			message: "there is no project prj_zzzzzzzzzz",
		});
		expect(html).toMatch(/<h1[^>]*>Não encontramos esta página<\/h1>/);
		expect(html.match(/<h1/g)).toHaveLength(1);
		expect(html).not.toMatch(/<h1[^>]*>[^<]*prj_/);
	});

	test("the text of the core stays under the technical details, as text", () => {
		const html = page({
			status: 422,
			message: "Items <b>would</b> overlap on track trk_1.",
			code: "E_OVERLAP",
			hint: "move the item & try again",
		});
		expect(html).toContain("Detalhes técnicos");
		expect(html).toContain(
			"Items &lt;b&gt;would&lt;/b&gt; overlap on track trk_1.",
		);
		expect(html).toContain("move the item &amp; try again");
		expect(html).toContain("E_OVERLAP");
		expect(html).not.toContain("<b>");
	});

	test("says how to fix it, in two numbered steps, by the kind of error", () => {
		const missing = page({ status: 404, message: "Not Found" });
		expect(missing).toContain("Como resolver:");
		expect(missing).toContain("Volte aos projetos");
		expect(missing).toContain("Olhe a lixeira");
		expect(missing.match(/data-slot="numbered-step"/g)).toHaveLength(2);
		expect(page({ status: 403, message: "Forbidden" })).toContain(
			"Você não tem acesso a esta página",
		);
		expect(page({ status: 500, message: "Internal Server Error" })).toContain(
			"O Stuarty não conseguiu terminar isto",
		);
		expect(page({ status: 422, message: "x" })).toContain(
			"Algo impediu esta página de abrir",
		);
	});

	test("offers the way back and, for a missing page, the trash", () => {
		const html = page({ status: 404, message: "Not Found" });
		expect(html).toContain('href="/projects"');
		expect(html).toContain('href="/projects#project-trash"');
		expect(html).toContain("Abrir a lixeira");
		expect(html).toContain("Erro 404");
		expect(page({ status: 500, message: "x" })).toContain('href="/"');
	});

	test("shows no hint or code that the server did not send", () => {
		const html = page({ status: 500, message: "Internal Server Error" });
		expect(html).not.toContain("font-mono");
	});
});
