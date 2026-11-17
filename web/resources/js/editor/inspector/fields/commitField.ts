// The commit rules of SDD-012 section 12.3 as a small state machine, so that
// they can be tested without a DOM: one command per committed change, never
// per keystroke. The React hook around it is useCommitField.ts.

export interface FieldError {
	code: string;
	message: string;
	hint?: string;
	// The `details` of the core's error, as received.
	details?: { readonly [key: string]: unknown };
}

export type CommitResult = { ok: true } | { ok: false; error: FieldError };

export interface FieldTimers {
	set(run: () => void, ms: number): unknown;
	clear(handle: unknown): void;
}

export const COALESCE_MS = 400;

export interface CommitFieldOptions {
	// Sends the draft; the owner of the field turns it into operations.
	commit(draft: string): Promise<CommitResult>;
	timers?: FieldTimers;
}

export interface FieldSnapshot {
	// What the field shows: the draft while there is one, else the stored value.
	text: string;
	drafting: boolean;
	// A command is in flight: aria-busy, the draft is kept.
	busy: boolean;
	error: FieldError | null;
	// Someone else changed the stored value while this field had a draft.
	remoteActor: string | null;
}

const browserTimers: FieldTimers = {
	set: (run, ms) => setTimeout(run, ms),
	clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
};

export class CommitField {
	private stored: string;
	private draft: string | null = null;
	private busy = false;
	private error: FieldError | null = null;
	private focused = false;
	private remoteActor: string | null = null;
	private timer: unknown = null;
	private shot: FieldSnapshot;
	private readonly listeners = new Set<() => void>();
	private readonly timers: FieldTimers;

	constructor(
		stored: string,
		private readonly options: CommitFieldOptions,
	) {
		this.stored = stored;
		this.timers = options.timers ?? browserTimers;
		this.shot = this.build();
	}

	private build(): FieldSnapshot {
		return {
			text: this.draft ?? this.stored,
			drafting: this.draft !== null,
			busy: this.busy,
			error: this.error,
			remoteActor: this.remoteActor,
		};
	}

	private publish(): void {
		this.shot = this.build();
		for (const listener of this.listeners) listener();
	}

	readonly subscribe = (listener: () => void): (() => void) => {
		this.listeners.add(listener);
		return () => this.listeners.delete(listener);
	};

	readonly snapshot = (): FieldSnapshot => this.shot;

	// The value of the read model changed (an edit of this tab, an event of
	// another actor). A field being edited keeps its draft and notes who changed
	// the value; the conflict policy of section 4.5 decides on commit.
	setStored(stored: string, remoteActor: string | null): void {
		if (stored === this.stored) return;
		this.stored = stored;
		if (this.draft !== null && this.focused && !this.busy) {
			this.remoteActor = remoteActor;
		} else if (!this.busy) {
			this.draft = null;
			this.remoteActor = null;
		}
		this.publish();
	}

	focus(): void {
		this.focused = true;
	}

	// A change of text. A burst (arrow keys, spin buttons) is coalesced: one
	// command with the final value, 400 ms after the last change.
	change(text: string, burst = false): void {
		this.draft = text;
		this.error = null;
		this.cancelTimer();
		if (burst) {
			this.timer = this.timers.set(() => {
				this.timer = null;
				void this.commit();
			}, COALESCE_MS);
		}
		this.publish();
	}

	private cancelTimer(): void {
		if (this.timer !== null) this.timers.clear(this.timer);
		this.timer = null;
	}

	// Enter, blur, a slider release or a select. A draft equal to the stored
	// value sends nothing.
	async commit(): Promise<void> {
		this.cancelTimer();
		const draft = this.draft;
		if (draft === null) return;
		if (draft === this.stored) {
			this.draft = null;
			this.remoteActor = null;
			this.publish();
			return;
		}
		if (this.busy) return;
		this.busy = true;
		this.publish();
		const result = await this.options.commit(draft);
		this.busy = false;
		this.error = result.ok ? null : result.error;
		// A change made while the command was in flight is a step the person
		// took (a key pressed again): keep it and send it next, never drop it.
		if (this.draft !== null && this.draft !== draft && result.ok) {
			this.publish();
			void this.commit();
			return;
		}
		// On success the draft gives way to the read model, on error the stored
		// value returns and the error shows under the field.
		this.draft = null;
		this.remoteActor = null;
		this.publish();
	}

	// Esc: the draft is discarded and nothing is sent.
	revert(): void {
		this.cancelTimer();
		this.draft = null;
		this.error = null;
		this.remoteActor = null;
		this.publish();
	}

	blur(): Promise<void> {
		this.focused = false;
		return this.commit();
	}
}
