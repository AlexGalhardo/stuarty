import { expect, seedProject, test } from "./helpers";

interface EditBody {
	operations: { op: string; args: Record<string, unknown> }[];
}

// P2-UI-005 (SDD-012 section 12.3): one command per committed change, never per
// keystroke. The arrow-key burst and the slider release are covered by the
// unit tests of the commit rules until a number field and a slider are in a
// panel (the transform section).
test.describe("inspector-debounce", () => {
	test("typing a time sends one item.move on Enter, Esc sends none, a bad time is not sent", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const sent: EditBody["operations"] = [];
		page.on("request", (r) => {
			if (r.method() !== "POST" || !r.url().endsWith("/edit")) return;
			const body = r.postDataJSON() as EditBody;
			sent.push(...body.operations);
		});

		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await expect(page.getByRole("heading", { name: "Project" })).toBeVisible();
		await page.locator("[data-item-id]").click();
		await expect(page.getByRole("heading", { name: /^Clip/ })).toBeVisible();

		const start = page.getByLabel("Start", { exact: true });
		await start.fill("");
		await start.pressSequentially("1.5");
		await start.press("s");
		expect(sent).toHaveLength(0);
		await start.press("Enter");
		await expect.poll(() => sent.length).toBe(1);
		expect(sent[0]).toMatchObject({
			op: "item.move",
			args: { start: "1.5s" },
		});
		await expect(start).toHaveValue("00:01.500");

		await start.fill("2");
		await start.press("Escape");
		await expect(start).toHaveValue("00:01.500");
		await start.fill("abc");
		await start.press("Enter");
		await expect(
			page.getByRole("alert").filter({ hasText: "Invalid time" }),
		).toBeVisible();
		await page.waitForTimeout(300);
		expect(sent).toHaveLength(1);

		const name = page.getByLabel("Name", { exact: true });
		await name.fill("Intro");
		await name.press("Enter");
		await expect.poll(() => sent.length).toBe(2);
		expect(sent[1]).toMatchObject({
			op: "item.rename",
			args: { name: "Intro" },
		});
		await expect(
			page.getByRole("heading", { name: /Clip.*Intro/ }),
		).toBeVisible();
	});
});
