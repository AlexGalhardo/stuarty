// A path as the user pastes it: surrounding double quotes (Windows "Copy as
// path") are stripped and spaces trimmed; nothing else is interpreted. Several
// lines import each line in order (SDD-012 section 13.1).
export function parsePastedPaths(text: string): string[] {
	return text
		.split(/\r?\n/)
		.map((line) => line.trim())
		.map((line) =>
			line.length >= 2 && line.startsWith('"') && line.endsWith('"')
				? line.slice(1, -1).trim()
				: line,
		)
		.filter((line) => line.length > 0);
}

// Runs imports one after the other. Paths added while one runs wait their
// turn, so the caller never has to lock the input (SDD-012 section 13.1).
export interface ImportQueue {
	add(paths: readonly string[]): void;
	readonly busy: boolean;
}

export function createImportQueue(
	run: (paths: readonly string[]) => Promise<void>,
	onBusy: (busy: boolean) => void,
): ImportQueue {
	const waiting: (readonly string[])[] = [];
	let draining = false;

	async function drain(): Promise<void> {
		draining = true;
		onBusy(true);
		try {
			for (let next = waiting.shift(); next; next = waiting.shift()) {
				try {
					await run(next);
				} catch {
					// The runtime reports import failures in the library state.
				}
			}
		} finally {
			draining = false;
			onBusy(false);
		}
	}

	return {
		add(paths) {
			if (paths.length === 0) return;
			waiting.push(paths);
			if (!draining) void drain();
		},
		get busy() {
			return draining;
		},
	};
}
