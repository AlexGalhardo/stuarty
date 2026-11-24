import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { ApiError } from "../editor/api/errors";
import { devFixture } from "../editor/boot/devFixture";
import type { ImageApi } from "./api/imageApi";
import ErrorBar from "./ErrorBar";
import { ImageEditorProvider } from "./state/context";
import { createImageStore } from "./state/store";

const messages = devFixture(1).bootstrap.messages;

function render(error: ApiError | null): string {
	const api = {} as ImageApi;
	const store = createImageStore(api, "img_aaaaaaaaaa");
	store.setState({ error });
	// A server render reads the initial state of a zustand store; the test asks for the one it just set.
	store.getInitialState = () => store.getState();
	return renderToString(
		<I18nProvider messages={messages} locale="en">
			<ImageEditorProvider store={store} api={api}>
				<ErrorBar />
			</ImageEditorProvider>
		</I18nProvider>,
	);
}

describe("image editor ErrorBar", () => {
	test("draws nothing while there is no error", () => {
		expect(render(null)).toBe("");
	});

	test("says the error with an icon and the words of the core, never colour alone", () => {
		const html = render(
			new ApiError(409, {
				code: "E_CONFLICT",
				message: "The layer changed since you read it.",
				hint: "Read the image again.",
			}),
		);
		expect(html).toContain('role="alert"');
		expect(html).toContain("The layer changed since you read it.");
		expect(html).toContain("Read the image again.");
		// the icon is decoration; the dismiss button has a name
		expect(html).toMatch(/<svg[^>]*aria-hidden="true"/);
		expect(html).toContain('aria-label="Dismiss the error"');
	});

	test("draws the text of the error as text, not as markup", () => {
		const html = render(
			new ApiError(400, {
				code: "E_ARGS",
				message: "<img src=x onerror=boom>",
			}),
		);
		expect(html).not.toContain("<img");
		expect(html).toContain("&lt;img");
	});
});
