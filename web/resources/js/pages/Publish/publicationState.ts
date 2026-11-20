import {
	PUBLICATION_EVENT,
	type Publication,
	type Target,
	type TargetState,
} from "./types";

// What a publication event does to the list on screen: the publication it carries replaces the one with its id, and
// is not added when the list does not hold it (the page reads the list again, and the service says whether the
// publication belongs to the range or the filter).
export function applyPublicationEvent(
	was: readonly Publication[],
	name: string | null,
	data: string,
): { next: Publication[]; known: boolean } {
	if (name !== PUBLICATION_EVENT) return { next: [...was], known: true };
	let parsed: unknown;
	try {
		parsed = JSON.parse(data);
	} catch {
		return { next: [...was], known: true };
	}
	if (!isPublication(parsed)) return { next: [...was], known: true };
	const incoming = parsed;
	const known = was.some((p) => p.id === incoming.id);
	return {
		next: was.map((p) => (p.id === incoming.id ? incoming : p)),
		known,
	};
}

export function isPublication(value: unknown): value is Publication {
	if (typeof value !== "object" || value === null) return false;
	const record = value as Record<string, unknown>;
	return (
		typeof record.id === "string" &&
		typeof record.project_id === "string" &&
		typeof record.title === "string" &&
		Array.isArray(record.targets)
	);
}

// The buttons a target offers in its state. The service refuses the ones it does not allow; this is only what the
// page shows, so a person is not offered a button that cannot work.
export interface TargetActions {
	schedule: boolean;
	sendNow: boolean;
	retry: boolean;
	cancel: boolean;
	// the target waits for a person: the page links to the approval queue
	approve: boolean;
	// its fields can be edited
	edit: boolean;
}

const NONE: TargetActions = {
	schedule: false,
	sendNow: false,
	retry: false,
	cancel: false,
	approve: false,
	edit: false,
};

export function actionsOf(state: TargetState): TargetActions {
	switch (state) {
		case "draft":
			return {
				...NONE,
				schedule: true,
				sendNow: true,
				cancel: true,
				edit: true,
			};
		case "waiting_approval":
			return {
				...NONE,
				approve: true,
				schedule: true,
				cancel: true,
				edit: true,
			};
		case "approved":
		case "scheduled":
			return {
				...NONE,
				schedule: true,
				sendNow: true,
				cancel: true,
				edit: true,
			};
		case "failed":
			return { ...NONE, retry: true, schedule: true, cancel: true, edit: true };
		case "missed":
			return {
				...NONE,
				sendNow: true,
				schedule: true,
				cancel: true,
				edit: true,
			};
		case "publishing":
		case "published":
		case "sent_as_draft":
		case "cancelled":
			return NONE;
	}
}

export function targetOf(
	publications: readonly Publication[],
	id: string,
): { publication: Publication; target: Target } | null {
	for (const publication of publications) {
		const target = publication.targets.find((t) => t.id === id);
		if (target !== undefined) return { publication, target };
	}
	return null;
}

// An address a network gave is a link only when it is an https address; anything else is text.
export function linkOf(url: string | undefined): string | null {
	if (url === undefined) return null;
	try {
		const parsed = new URL(url);
		return parsed.protocol === "https:" ? parsed.toString() : null;
	} catch {
		return null;
	}
}
