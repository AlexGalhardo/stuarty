import { useCallback, useEffect, useMemo, useState } from "react";

import type { Translate } from "@/lib/i18n";
import { ApiError } from "../../api/errors";
import type { CommandSummary } from "../../api/types";
import { useRuntime } from "../../state/EditorContext";

export interface PaletteRow {
	command: CommandSummary;
	// The localized title, or the English summary when the catalogue has none.
	title: string;
	translated: boolean;
}

// SDD-012 section 9.2: `ui.editor.commands.<name>.title`, falling back to the
// summary of the registry.
export function commandRow(t: Translate, command: CommandSummary): PaletteRow {
	const key = `ui.editor.commands.${command.name}.title`;
	const title = t(key);
	return title === key
		? { command, title: command.summary, translated: false }
		: { command, title, translated: true };
}

const fold = (text: string): string =>
	text.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();

// The rows of the list: internal commands are hidden, the order is alphabetical
// by title, and a query keeps the rows whose title, name or summary contain
// every word of it.
export function paletteRows(
	t: Translate,
	locale: string,
	commands: readonly CommandSummary[],
	query: string,
): PaletteRow[] {
	const words = fold(query).split(/\s+/).filter(Boolean);
	return commands
		.filter((command) => command.internal !== true)
		.map((command) => commandRow(t, command))
		.filter((row) => {
			if (words.length === 0) return true;
			const haystack = fold(
				`${row.title} ${row.command.name} ${row.command.summary}`,
			);
			return words.every((word) => haystack.includes(word));
		})
		.sort(
			(a, b) =>
				a.title.localeCompare(b.title, locale) ||
				a.command.name.localeCompare(b.command.name),
		);
}

// The part of a title to show in bold: where the whole query first appears.
export function matchRange(
	title: string,
	query: string,
): [number, number] | null {
	const needle = fold(query).trim();
	if (needle === "") return null;
	// Folding removes marks, so the offsets are only valid when the length holds.
	const folded = fold(title);
	if (folded.length !== title.length) return null;
	const at = folded.indexOf(needle);
	return at < 0 ? null : [at, at + needle.length];
}

export type RegistryState =
	| { status: "loading" }
	| { status: "ready"; commands: CommandSummary[] }
	| { status: "failed"; error: ApiError };

// The live registry (never a hard-coded list): fetched when the palette first
// opens, again on a retry.
export function useRegistry(open: boolean): {
	state: RegistryState;
	reload(): void;
} {
	const runtime = useRuntime();
	const [state, setState] = useState<RegistryState>({ status: "loading" });
	const status = state.status;

	useEffect(() => {
		if (!open || status !== "loading") return;
		let current = true;
		runtime.client
			.getCommands()
			.then((result) => {
				if (current)
					setState({ status: "ready", commands: result.commands ?? [] });
			})
			.catch((error: unknown) => {
				if (current && error instanceof ApiError)
					setState({ status: "failed", error });
			});
		return () => {
			current = false;
		};
	}, [open, status, runtime]);

	const reload = useCallback(() => setState({ status: "loading" }), []);
	return { state, reload };
}

export function usePaletteSearch(
	t: Translate,
	locale: string,
	state: RegistryState,
	query: string,
): PaletteRow[] {
	return useMemo(
		() =>
			state.status === "ready"
				? paletteRows(t, locale, state.commands, query)
				: [],
		[t, locale, state, query],
	);
}
