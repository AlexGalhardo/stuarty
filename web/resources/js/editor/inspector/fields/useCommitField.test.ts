import { describe, expect, test } from "vitest";

import {
	COALESCE_MS,
	CommitField,
	type CommitResult,
	type FieldTimers,
} from "./commitField";

// Timers the test advances by hand.
class ManualTimers implements FieldTimers {
	private now = 0;
	private next = 1;
	private readonly pending = new Map<number, { at: number; run: () => void }>();

	set(run: () => void, ms: number): number {
		const id = this.next++;
		this.pending.set(id, { at: this.now + ms, run });
		return id;
	}

	clear(handle: unknown): void {
		this.pending.delete(handle as number);
	}

	advance(ms: number): void {
		this.now += ms;
		for (const [id, timer] of [...this.pending]) {
			if (timer.at <= this.now) {
				this.pending.delete(id);
				timer.run();
			}
		}
	}
}

function setup(stored = "1s") {
	const sent: string[] = [];
	const timers = new ManualTimers();
	let result: CommitResult = { ok: true };
	const field = new CommitField(stored, {
		timers,
		commit: async (draft) => {
			sent.push(draft);
			return result;
		},
	});
	return {
		field,
		sent,
		timers,
		fail: (code: string) => {
			result = { ok: false, error: { code, message: "no" } };
		},
	};
}

describe("CommitField (SDD-012 section 12.3)", () => {
	test("a draft equal to the stored value sends nothing", async () => {
		const { field, sent } = setup("1s");
		field.change("1s");
		await field.commit();
		field.change("2s");
		field.change("1s");
		await field.blur();
		expect(sent).toEqual([]);
		expect(field.snapshot().drafting).toBe(false);
	});

	test("Enter commits once; Esc discards without a command", async () => {
		const { field, sent } = setup();
		field.change("1.5s");
		await field.commit();
		expect(sent).toEqual(["1.5s"]);
		field.change("9s");
		field.revert();
		await field.blur();
		expect(sent).toEqual(["1.5s"]);
		expect(field.snapshot().text).toBe("1s");
	});

	test("blur with a changed value commits", async () => {
		const { field, sent } = setup();
		field.focus();
		field.change("3s");
		await field.blur();
		expect(sent).toEqual(["3s"]);
	});

	test("an arrow burst is one command with the final value, 400 ms after the last change", async () => {
		const { field, sent, timers } = setup("1");
		for (const text of ["2", "3", "4", "5"]) {
			field.change(text, true);
			timers.advance(COALESCE_MS - 1);
		}
		expect(sent).toEqual([]);
		timers.advance(1);
		await Promise.resolve();
		expect(sent).toEqual(["5"]);
	});

	test("a remote change keeps the draft and names the actor", async () => {
		const { field, sent } = setup("1s");
		field.focus();
		field.change("7s");
		field.setStored("4s", "claude-code");
		const shown = field.snapshot();
		expect(shown.text).toBe("7s");
		expect(shown.remoteActor).toBe("claude-code");
		await field.blur();
		expect(sent).toEqual(["7s"]);
	});

	test("a remote change with no draft just shows the new value", () => {
		const { field } = setup("1s");
		field.setStored("4s", "claude-code");
		expect(field.snapshot()).toMatchObject({
			text: "4s",
			drafting: false,
			remoteActor: null,
		});
	});

	test("on error the draft is dropped and the error shows", async () => {
		const { field, fail } = setup();
		fail("E_INVALID_TIME");
		field.change("x");
		await field.commit();
		expect(field.snapshot()).toMatchObject({
			text: "1s",
			error: { code: "E_INVALID_TIME" },
			busy: false,
		});
	});

	test("the field is busy while the command is in flight and keeps the draft", async () => {
		let release: (r: CommitResult) => void = () => undefined;
		const field = new CommitField("1s", {
			commit: () =>
				new Promise<CommitResult>((resolve) => {
					release = resolve;
				}),
		});
		field.change("2s");
		const done = field.commit();
		expect(field.snapshot()).toMatchObject({ busy: true, text: "2s" });
		release({ ok: true });
		await done;
		expect(field.snapshot().busy).toBe(false);
	});

	// VV-182: a step taken while an earlier command is in flight is sent next,
	// not dropped when the earlier one answers.
	test("a change made while a command is in flight is sent afterwards", async () => {
		const sent: string[] = [];
		const releases: (() => void)[] = [];
		const field = new CommitField("0", {
			commit: (draft) =>
				new Promise<CommitResult>((resolve) => {
					sent.push(draft);
					releases.push(() => resolve({ ok: true }));
				}),
		});
		field.change("1");
		const first = field.commit();
		field.change("2", true);
		releases[0]?.();
		await first;
		await Promise.resolve();
		expect(sent).toEqual(["1", "2"]);
		expect(field.snapshot().text).toBe("2");
		releases[1]?.();
		await Promise.resolve();
		await Promise.resolve();
		expect(field.snapshot()).toMatchObject({ busy: false, drafting: false });
	});
});
