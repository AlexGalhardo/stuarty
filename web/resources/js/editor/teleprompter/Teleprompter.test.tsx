import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import en from "../../../lang/en.json";
import type { TextItem, WritingClient } from "../../pages/Write/client";
import { DEFAULT_PREFS, type TeleprompterPrefs } from "./prefs";
import { loadScript } from "./script";
import { NoScript, TeleprompterView } from "./Teleprompter";

function flat(
	tree: Record<string, unknown>,
	prefix: string,
	out: Record<string, string> = {},
): Record<string, string> {
	for (const [key, value] of Object.entries(tree)) {
		if (typeof value === "string") out[`${prefix}${key}`] = value;
		else flat(value as Record<string, unknown>, `${prefix}${key}.`, out);
	}
	return out;
}
const messages = flat(en.ui, "ui.");

const text = (id: string, title: string): TextItem => ({
	id,
	project_id: "prj_1",
	title,
	text_kind: "script",
	state: "active",
	words: 3,
	reading_minutes: 1,
	revision: 2,
	updated_at: "2026-11-25T00:00:00Z",
});

function fakeClient(
	texts: TextItem[],
	body = "Hello there",
): {
	client: Pick<WritingClient, "list" | "get">;
	list: ReturnType<typeof vi.fn>;
	get: ReturnType<typeof vi.fn>;
} {
	const list = vi.fn(async () => ({ texts }));
	const get = vi.fn(async (_project: string, id: string) => ({
		project_id: "prj_1",
		item: texts.find((item) => item.id === id) as TextItem,
		revision: {
			revision: 2,
			actor: "local:ui",
			source: "write" as const,
			created_at: "2026-11-25T00:00:00Z",
		},
		body,
	}));
	return { client: { list, get } as never, list, get };
}

function draw(
	body: string,
	prefs: Partial<TeleprompterPrefs> = {},
	playing = false,
): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en">
			<TeleprompterView
				script={{ itemId: "itm_1", title: "Launch script", body }}
				prefs={{ ...DEFAULT_PREFS, ...prefs }}
				playing={playing}
				onToggle={() => {}}
				onRestart={() => {}}
				onChange={() => {}}
			/>
		</I18nProvider>,
	);
}

describe("the script of the teleprompter", () => {
	test("is the newest active script of the project, read with the writing operations that exist", async () => {
		const { client, list, get } = fakeClient([
			text("itm_new", "Newest"),
			text("itm_old", "Older"),
		]);
		const script = await loadScript(client, "prj_1");
		expect(script).toEqual({
			itemId: "itm_new",
			title: "Newest",
			body: "Hello there",
		});
		expect(list).toHaveBeenCalledWith({
			project_id: "prj_1",
			text_kind: "script",
			state: "active",
		});
		expect(get).toHaveBeenCalledWith("prj_1", "itm_new");
	});

	test("is none when the project has no script", async () => {
		const { client, get } = fakeClient([]);
		expect(await loadScript(client, "prj_1")).toBeNull();
		expect(get).not.toHaveBeenCalled();
	});
});

describe("the teleprompter", () => {
	test("shows the script of the project", () => {
		const html = draw("# Hook\n\nWelcome to the channel.");
		expect(html).toContain("Welcome to the channel.");
		expect(html).toContain("# Hook");
		expect(html).toContain('data-slot="teleprompter-text"');
	});

	test("draws a script that carries markup as text", () => {
		const html = draw(
			'<img src=x onerror="alert(1)"><script>alert(2)</script>',
		);
		expect(html).not.toContain("<img");
		expect(html).not.toContain("<script>alert");
		expect(html).toContain("&lt;img src=x onerror=");
	});

	test("applies the size and the mirror the person chose", () => {
		const plain = draw("Hi");
		expect(plain).toContain("font-size:48px");
		expect(plain).toContain('data-mirror="false"');
		expect(plain).not.toContain("scale:-1 1");

		const big = draw("Hi", { size: 96, mirror: true });
		expect(big).toContain("font-size:96px");
		expect(big).toContain('data-mirror="true"');
		expect(big).toContain("scale:-1 1");
	});

	test("shows the speed and the state of the scroll", () => {
		expect(draw("Hi", { speed: 140 })).toContain("140");
		expect(draw("Hi", {}, false)).toContain("Start");
		expect(draw("Hi", {}, true)).toContain("Pause");
		expect(draw("Hi", {}, true)).toContain('aria-pressed="true"');
	});

	test("says there is no script, and what to do", () => {
		const html = renderToString(
			<I18nProvider messages={messages} locale="en">
				<NoScript />
			</I18nProvider>,
		);
		expect(html).toContain("This project has no script yet");
		expect(html).toContain('data-slot="teleprompter-empty"');
	});
});
