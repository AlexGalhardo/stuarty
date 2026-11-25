// Words for a request of the queue. The service sends a technical English summary and ids ("con_...",
// "prj_..."); a person reads the kind of action, the item and the name of the place. This only words what the
// service reported: it decides nothing and invents no fact (a missing name reads as a generic word).

type Translate = (
	key: string,
	params?: Record<string, string | number>,
) => string;

export type NameOf = (id: string) => string | null | undefined;

// The place of a request, from the "connection / project" string of the inbox, as the names the person gave. An
// id with no known name is dropped, so a raw id never reaches the screen.
export function placeOf(
	destination: string,
	connection: NameOf,
	project: NameOf,
): string {
	const names: string[] = [];
	for (const part of destination.split("/")) {
		const id = part.trim();
		if (id === "") continue;
		const name = id.startsWith("con_") ? connection(id) : project(id);
		if (typeof name === "string" && name !== "") names.push(name);
	}
	return names.join(" / ");
}

// "Publish {item} to {destination}" in the person's language, from the kind of the job the service named.
export function headline(
	t: Translate,
	kind: string,
	item: string | undefined,
	destination: string | null | undefined,
): string {
	const name = kind.toLowerCase();
	const group = name.includes("publish")
		? "publish"
		: name.includes("generate")
			? "generate"
			: "other";
	if (group === "other") return t("ui.approvals.headline.other", { kind });
	return t(`ui.approvals.headline.${group}`, {
		item:
			item === undefined || item === ""
				? t("ui.approvals.headline.this_item")
				: `“${item}”`,
		destination:
			destination === undefined || destination === null || destination === ""
				? t("ui.approvals.headline.your_destination")
				: destination,
	});
}
