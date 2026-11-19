import { describe, expect, it } from "vitest";

import {
	addDays,
	addMonths,
	monthDays,
	onDay,
	placeByDay,
	queryRange,
	shiftLocal,
	wallOf,
	weekDays,
} from "./calendar";

describe("wallOf", () => {
	it("reads an instant on the wall clock of a zone", () => {
		expect(wallOf("2026-11-20T12:00:00Z", "America/Sao_Paulo")).toEqual({
			day: "2026-11-20",
			local: "2026-11-20T09:00",
			hour: 9,
			minute: 0,
		});
	});

	it("moves the day with the zone", () => {
		expect(wallOf("2026-11-20T01:30:00Z", "America/Sao_Paulo")?.day).toBe(
			"2026-11-19",
		);
		expect(wallOf("2026-11-20T23:30:00Z", "Asia/Tokyo")?.day).toBe(
			"2026-11-21",
		);
	});

	it("follows the change of the clock", () => {
		expect(wallOf("2027-03-14T07:30:00Z", "America/New_York")?.local).toBe(
			"2027-03-14T03:30",
		);
	});

	it("gives nothing for what is not a time or a zone", () => {
		expect(wallOf("soon", "UTC")).toBeNull();
		expect(wallOf("2026-11-20T12:00:00Z", "Mars/Base")).toBeNull();
	});
});

describe("days", () => {
	it("adds days and months across the ends", () => {
		expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
		expect(addDays("2026-03-01", -1)).toBe("2026-02-28");
		expect(addMonths("2026-11-20", 1)).toBe("2026-12-01");
		expect(addMonths("2026-01-31", -1)).toBe("2025-12-01");
	});

	it("shows six weeks of a month from the first day of the week", () => {
		const sunday = monthDays("2026-11-20", 0);
		expect(sunday).toHaveLength(42);
		expect(sunday[0]?.key).toBe("2026-11-01");
		expect(sunday[0]?.inMonth).toBe(true);
		expect(sunday[41]?.key).toBe("2026-12-12");
		expect(sunday[41]?.inMonth).toBe(false);

		const monday = monthDays("2026-11-20", 1);
		expect(monday[0]?.key).toBe("2026-10-26");
		expect(monday[0]?.inMonth).toBe(false);
	});

	it("shows a week", () => {
		const week = weekDays("2026-11-20", 1);
		expect(week.map((d) => d.key)).toEqual([
			"2026-11-16",
			"2026-11-17",
			"2026-11-18",
			"2026-11-19",
			"2026-11-20",
			"2026-11-21",
			"2026-11-22",
		]);
	});

	it("asks for a day more on each side", () => {
		const range = queryRange(weekDays("2026-11-20", 1));
		expect(range).toEqual({
			from: "2026-11-15T00:00:00Z",
			to: "2026-11-24T00:00:00Z",
		});
	});
});

describe("shiftLocal", () => {
	it("moves the wall clock by days and hours", () => {
		expect(shiftLocal("2026-11-20T09:00", 1, 0)).toBe("2026-11-21T09:00");
		expect(shiftLocal("2026-11-20T09:00", -1, 0)).toBe("2026-11-19T09:00");
		expect(shiftLocal("2026-11-20T09:00", 0, 1)).toBe("2026-11-20T10:00");
		expect(shiftLocal("2026-11-20T23:30", 0, 1)).toBe("2026-11-21T00:30");
		expect(shiftLocal("2026-11-30T09:15", 2, 0)).toBe("2026-12-02T09:15");
	});

	it("drops a time on another day at its own hour", () => {
		expect(onDay("2026-11-25", "2026-11-20T09:45")).toBe("2026-11-25T09:45");
	});
});

describe("placeByDay", () => {
	it("groups by the day in the zone and orders by time", () => {
		const items = [
			{ id: "b", at: "2026-11-20T15:00:00Z" },
			{ id: "a", at: "2026-11-20T12:00:00Z" },
			{ id: "c", at: "2026-11-21T12:00:00Z" },
			{ id: "none", at: null },
		];
		const placed = placeByDay(items, (i) => i.at, "America/Sao_Paulo");
		expect(placed.get("2026-11-20")?.map((p) => p.item.id)).toEqual(["a", "b"]);
		expect(placed.get("2026-11-21")?.map((p) => p.item.id)).toEqual(["c"]);
		expect(placed.size).toBe(2);
	});
});
