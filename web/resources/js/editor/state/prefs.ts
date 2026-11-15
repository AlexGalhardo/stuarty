// The only file that touches the storage of the browser (rule R7). A blocked
// storage (private mode) leaves the defaults and the choice lasts for the page.

export type TimeUnit = "s" | "f";

// The unit appended to a bare number typed in a time field (SDD-012 section 12.2).
export function readTimeUnit(): TimeUnit {
	try {
		return localStorage.getItem("vv.timeUnit") === "f" ? "f" : "s";
	} catch {
		return "s";
	}
}

export function writeTimeUnit(unit: TimeUnit): void {
	try {
		localStorage.setItem("vv.timeUnit", unit);
	} catch {
		// Storage can be blocked: the choice lasts for this page only.
	}
}

// The shortcut overrides (SDD-012 section 18.2): a JSON object from binding id
// to key. Anything that is not an object of strings reads as no overrides.
export function readKeymap(): Record<string, string> {
	try {
		const parsed: unknown = JSON.parse(
			localStorage.getItem("vv.keymap") ?? "null",
		);
		if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed))
			return {};
		const out: Record<string, string> = {};
		for (const [id, key] of Object.entries(parsed)) {
			if (typeof key === "string") out[id] = key;
		}
		return out;
	} catch {
		return {};
	}
}

export function writeKeymap(overrides: Readonly<Record<string, string>>): void {
	try {
		if (Object.keys(overrides).length === 0) {
			localStorage.removeItem("vv.keymap");
			return;
		}
		localStorage.setItem("vv.keymap", JSON.stringify(overrides));
	} catch {
		// Storage can be blocked: the bindings last for this page only.
	}
}
