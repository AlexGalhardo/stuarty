import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { ApiError } from "../api/errors";
import { FailureView } from "../ui/FailureView";
import { devFixture } from "./devFixture";
import { ErrorBoundary, failureOf } from "./ErrorBoundary";

const messages = devFixture(1).bootstrap.messages;

function render(node: React.ReactNode): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en">
			{node}
		</I18nProvider>,
	);
}

describe("failureOf", () => {
	test("keeps the code, the message and the hint of a core error", () => {
		const error = new ApiError(500, {
			code: "E_VALIDATION",
			message: "this project was saved by a newer version",
			hint: "update stuarty",
		});
		expect(failureOf(error)).toEqual({
			code: "E_VALIDATION",
			message: "this project was saved by a newer version",
			hint: "update stuarty",
		});
	});

	test("a script error has a message and no code", () => {
		expect(failureOf(new Error("boom"))).toEqual({ message: "boom" });
		expect(failureOf("text")).toEqual({ message: "text" });
	});
});

describe("FailureView", () => {
	test("shows the code, the message and the hint, escaped", () => {
		const html = render(
			<FailureView
				failure={{
					code: "E_VALIDATION",
					message: "newer <b>version</b>",
					hint: "update stuarty",
				}}
			/>,
		);
		expect(html).toContain("E_VALIDATION");
		expect(html).toContain("newer &lt;b&gt;version&lt;/b&gt;");
		expect(html).toContain("update stuarty");
	});
});

describe("ErrorBoundary", () => {
	test("renders its children when nothing throws", () => {
		expect(
			render(
				<ErrorBoundary>
					<p>fine</p>
				</ErrorBoundary>,
			),
		).toContain("fine");
	});

	test("a render error becomes the failure view, not a blank page", () => {
		const state = ErrorBoundary.getDerivedStateFromError(
			new ApiError(500, {
				code: "E_INTERNAL",
				message: "broke",
				hint: "retry",
			}),
		);
		expect(state.failure).toEqual({
			code: "E_INTERNAL",
			message: "broke",
			hint: "retry",
		});
		const boundary = new ErrorBoundary({ children: <p>child</p> });
		boundary.state = state;
		const html = render(boundary.render());
		expect(html).toContain('role="alert"');
		expect(html).toContain("E_INTERNAL");
		expect(html).toContain("broke");
		expect(html).toContain("retry");
		expect(html).not.toContain("child");
	});
});
