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
