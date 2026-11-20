// The message of a finding of the lint names its layer by id, which an agent needs and a person does not read
// (VV-325). The page says the name of the layer instead; with no name the id stays.
export function withLayerName(
	message: string,
	item: string | undefined,
	name: string | undefined,
): string {
	if (item === undefined || name === undefined || name.trim() === "") {
		return message;
	}
	return message.split(item).join(`“${name}”`);
}
