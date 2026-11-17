import { useCallback, useEffect, useMemo, useState } from "react";

import type { FontFamily, ImportedFont } from "../api/types";
import { useDocumentState, useRuntime } from "../state/EditorContext";

// The families a style may name: the bundled ones as `getFonts()` lists them
// first, then the fonts of the project (SDD-012 section 12.1). The browser
// never loads a font file: these are names for the select. `reload` asks the
// service again, for E_FONT_UNKNOWN.
export function useFonts(): {
	families: readonly FontFamily[];
	// The font files of the font store, by the hash `font.add` takes (VV-094).
	imported: readonly ImportedFont[];
	reload(): void;
} {
	const runtime = useRuntime();
	const projectFonts = useDocumentState((s) => s.document?.fonts);
	const [bundled, setBundled] = useState<readonly FontFamily[]>([]);
	const [imported, setImported] = useState<readonly ImportedFont[]>([]);
	const reload = useCallback(() => {
		runtime.client.getFonts().then(
			(result) => {
				setBundled(result.fonts);
				setImported(result.imported ?? []);
			},
			() => {},
		);
	}, [runtime]);
	useEffect(reload, [reload]);
	const families = useMemo(() => {
		const known = new Set(bundled.map((font) => font.family));
		const own = Object.entries(projectFonts ?? {})
			.filter(([family]) => !known.has(family))
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([family, font]) => ({
				family,
				weights: font.files.map((file) => file.weight),
			}));
		return [...bundled, ...own];
	}, [bundled, projectFonts]);
	return { families, imported, reload };
}
