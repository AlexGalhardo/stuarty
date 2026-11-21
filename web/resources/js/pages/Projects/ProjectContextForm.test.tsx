import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import type { Failure } from "@/components/Failure";
import { I18nProvider } from "@/lib/i18n";
import ProjectContextForm from "./ProjectContextForm";
import { emptyContext } from "./projectSchema";

function html(failure: Failure | null): string {
	return renderToString(
		<I18nProvider messages={{}} locale="en" locales={["en"]}>
			<ProjectContextForm
				initial={emptyContext()}
				submitLabel="Save"
				busy={false}
				failure={failure}
				onSubmit={() => undefined}
			/>
		</I18nProvider>,
	);
}

describe("what the core answered to the form", () => {
	test("an error that names a field shows under that field and marks it", () => {
		const out = html({
			code: "E_VALIDATION",
			message: "The due field is 2020-01-01, before today",
			path: "goal.due",
		});
		const input = out.indexOf('name="due"');
		const note = out.indexOf("The due field is 2020-01-01");
		expect(input).toBeGreaterThan(-1);
		expect(note).toBeGreaterThan(input);
		expect(out.slice(input - 400, input + 400)).toContain(
			'aria-invalid="true"',
		);
		expect(out.indexOf("The due field is 2020-01-01", note + 1)).toBe(-1);
	});

	test("an error about no field shows in the form, above the buttons", () => {
		const out = html({ code: "E_INTERNAL", message: "It did not work" });
		const note = out.indexOf("It did not work");
		expect(note).toBeGreaterThan(out.indexOf('name="due"'));
		expect(note).toBeLessThan(out.indexOf('type="submit"'));
	});

	test("no answer yet shows no alert", () => {
		expect(html(null)).not.toContain('role="alert"');
	});
});
