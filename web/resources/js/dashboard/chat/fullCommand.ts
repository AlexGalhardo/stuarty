// The full command of a profile, as the panel shows it before it is saved and wherever it is
// listed (SDD-017 section 3): the program and every argument, each in quotes when it holds a
// blank or a quote, so that what will run on the machine can be read in full. It is a display; the
// service takes the program and the arguments as an array and no shell ever sees this text.
export function fullCommand(program: string, args: readonly string[]): string {
	return [program, ...args].map(quote).join(" ");
}

const NEEDS_QUOTES = /[\s"'\\]/;
const ESCAPED = /(["\\])/g;

function quote(part: string): string {
	if (part === "" || NEEDS_QUOTES.test(part)) {
		return `"${part.replace(ESCAPED, "\\$1")}"`;
	}
	return part;
}

// One argument per line: a blank line is not an argument.
export function parseArgs(text: string): string[] {
	return text
		.split(/\r?\n/)
		.map((line) => line.replace(/\s+$/, ""))
		.filter((line) => line.trim() !== "");
}
