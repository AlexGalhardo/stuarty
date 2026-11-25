// The service writes a size with a decimal point ("52.1 KB"). The page draws it with the decimal mark of the
// language, as it does the dates (VV-442). It only swaps the mark of a text: the number and its unit are the
// service's.
export function localizeSize(text: string, locale: string): string {
	let mark = ".";
	try {
		mark =
			new Intl.NumberFormat(locale)
				.formatToParts(1.1)
				.find((part) => part.type === "decimal")?.value ?? ".";
	} catch {
		return text;
	}
	return mark === "." ? text : text.replace(/(\d)\.(\d)/g, `$1${mark}$2`);
}
