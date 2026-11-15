// The pure part of check-i18n (SDD-012 section 16.4): flattening a catalogue
// and comparing two locales (rules I1 and I2). No syntax tree here, so Vitest
// can test it.

export type Catalog = { [key: string]: unknown };

export interface Leaf {
	key: string;
	// The plural categories when the leaf is a plural object, else null.
	categories: string[] | null;
	texts: string[];
}

export interface Problem {
	rule: string;
	message: string;
}

const PLURAL = new Set(["zero", "one", "two", "few", "many", "other"]);

function isPlural(value: { [key: string]: unknown }): boolean {
	const keys = Object.keys(value);
	return (
		keys.length > 0 &&
		keys.includes("other") &&
		keys.every((k) => PLURAL.has(k) && typeof value[k] === "string")
	);
}

// `ui.editor.activity.editing_others` is one leaf even though it holds the
// categories one and other.
export function flatten(catalog: Catalog, prefix = ""): Map<string, Leaf> {
	const out = new Map<string, Leaf>();
	for (const [name, value] of Object.entries(catalog)) {
		const key = prefix === "" ? name : `${prefix}.${name}`;
		if (typeof value === "string") {
			out.set(key, { key, categories: null, texts: [value] });
		} else if (typeof value === "object" && value !== null) {
			const dict = value as { [key: string]: unknown };
			if (isPlural(dict)) {
				out.set(key, {
					key,
					categories: Object.keys(dict),
					texts: Object.values(dict) as string[],
				});
			} else {
				for (const [k, leaf] of flatten(dict, key)) out.set(k, leaf);
			}
		}
	}
	return out;
}

export function placeholders(texts: readonly string[]): Set<string> {
	const names = new Set<string>();
	for (const text of texts) {
		for (const match of text.matchAll(/\{(\w+)\}/g)) names.add(match[1] ?? "");
	}
	return names;
}

// I1: the key sets are equal. I2: per key, the same placeholders, and a plural
// in one locale is a plural in the other (each with its own categories, since
// a locale needs only the ones its plural rules use).
export function compareCatalogs(
	en: Map<string, Leaf>,
	other: Map<string, Leaf>,
	otherName: string,
): Problem[] {
	const problems: Problem[] = [];
	for (const key of en.keys()) {
		if (!other.has(key)) {
			problems.push({ rule: "I1", message: `${key} is missing in ${otherName}` });
		}
	}
	for (const key of other.keys()) {
		if (!en.has(key)) {
			problems.push({ rule: "I1", message: `${key} is only in ${otherName}` });
		}
	}
	for (const [key, leaf] of en) {
		const twin = other.get(key);
		if (twin === undefined) continue;
		if ((leaf.categories === null) !== (twin.categories === null)) {
			problems.push({
				rule: "I2",
				message: `${key} is a plural in only one of en and ${otherName}`,
			});
			continue;
		}
		const a = [...placeholders(leaf.texts)].sort().join(",");
		const b = [...placeholders(twin.texts)].sort().join(",");
		if (a !== b) {
			problems.push({
				rule: "I2",
				message: `${key} has placeholders {${a}} in en and {${b}} in ${otherName}`,
			});
		}
	}
	return problems;
}
