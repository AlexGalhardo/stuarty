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
