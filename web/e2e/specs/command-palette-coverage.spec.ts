import { caller, expect, test } from "./helpers";

interface Summary {
	name: string;
	internal?: boolean;
}

// P2-UI-003: the palette is generated from the live registry, so it lists every
// command that is not internal, with no list of names in the UI.
test.describe("command-palette-coverage", () => {
	test("the palette lists every non-internal command of the registry", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			name: `Palette ${Date.now()}`,
		});
		const registry = (await call("GET", "/commands")) as {
			commands: Summary[];
		};
		const expected = registry.commands
			.filter((c) => c.internal !== true)
			.map((c) => c.name)
			.sort();
		expect(expected.length).toBeGreaterThan(40);

		await page.goto(`/editor/${created.project_id as string}`);
		await expect(
			page.getByRole("button", { name: /Command palette/ }),
		).toBeEnabled();
		await page.keyboard.press("Control+k");
		const options = page.getByRole("option");
		await expect(options).toHaveCount(expected.length);
		const listed = await options.evaluateAll((nodes) =>
			nodes.map((n) => (n as HTMLElement).dataset.command ?? ""),
		);
		expect([...listed].sort()).toEqual(expected);

		await page
			.getByRole("combobox", { name: "Search commands" })
			.fill("zzzzzz");
		await expect(page.getByText("No command matches")).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(page.getByRole("dialog")).toHaveCount(0);
	});
});

interface Detail {
	name: string;
	internal?: boolean;
	args_schema: Schema;
	errors: string[];
}

interface Schema {
	type?: string;
	properties?: Record<string, Schema>;
	additionalProperties?: unknown;
}

// A sub-schema the form cannot render as a control: no type, or an object with
// no named properties. The registry has a few; the form shows a JSON textarea
// for each (SDD-012 9.2) and the spec names them, so a new one is noticed.
function fallbackPaths(schema: Schema, path: string): string[] {
	if (schema.type === "object" && schema.properties !== undefined) {
		return Object.entries(schema.properties).flatMap(([name, sub]) =>
			fallbackPaths(sub, `${path}.${name}`),
		);
	}
	return schema.type === undefined || schema.type === "object" ? [path] : [];
}

test.describe("command-palette-coverage form", () => {
	test("every command opens a form with a control per property and its example is accepted as a dry run", async ({
		page,
		request,
	}) => {
		test.setTimeout(240_000);
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			name: `Palette forms ${Date.now()}`,
		});
		const registry = (await call("GET", "/commands")) as {
			commands: Summary[];
		};
		const names = registry.commands
			.filter((c) => c.internal !== true)
			.map((c) => c.name);

		await page.goto(`/editor/${created.project_id as string}`);
		await expect(
			page.getByRole("button", { name: /Command palette/ }),
		).toBeEnabled();
		await page.keyboard.press("Control+k");
		const search = page.getByRole("combobox", { name: "Search commands" });

		for (const name of names) {
			const detail = (await call("GET", `/commands/${name}`)) as {
				command: Detail;
			};
			const { args_schema: schema, errors } = detail.command;

			await search.fill(name);
			await page.locator(`[data-command="${name}"]`).click();
			const form = page.locator(`[data-form="${name}"]`);
			await expect(form, name).toBeVisible();

			for (const property of Object.keys(schema.properties ?? {})) {
				await expect(
					form.locator(`[data-path="args.${property}"]`),
					`${name}.${property}`,
				).toHaveCount(1);
			}
			const expected = fallbackPaths(schema, "args").filter(
				(p) => p !== "args",
			);
			const shown = await form
				.locator('[data-fallback="json"]')
				.evaluateAll((nodes) =>
					nodes.map((n) => (n as HTMLElement).dataset.path ?? ""),
				);
			expect(shown.sort(), name).toEqual(expected.sort());

			await page.getByRole("button", { name: "Fill example" }).click();
			const answered = page.waitForResponse(
				(r) => r.url().endsWith("/edit") && r.request().method() === "POST",
			);
			await page.getByRole("button", { name: "Preview", exact: true }).click();
			const response = await answered;
			const body = (await response.json()) as {
				applied?: boolean;
				code?: string;
				path?: string;
			};
			if (response.ok()) {
				expect(body.applied, name).toBe(false);
			} else {
				expect(errors, name).toContain(body.code);
				// caption.from_transcript and caption.retime_from_clip need `resolved`,
				// which the service fills from a transcript that this project lacks; the
				// core reports that as E_VALIDATION on `args.resolved`.
				if (body.path !== "args.resolved") {
					expect(body.code, `${name} ${JSON.stringify(body)}`).not.toBe(
						"E_VALIDATION",
					);
				}
				expect(body.code, name).not.toBe("E_UNKNOWN_COMMAND");
			}

			await page.getByRole("button", { name: "Back to the list" }).click();
		}
	});
});
