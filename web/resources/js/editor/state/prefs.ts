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
