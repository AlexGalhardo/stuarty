import type {
	Connection,
	SocialAccount,
	SocialNetwork,
} from "../../editor/api/types";

// The kind of content a publication holds: a blog post is text, a video is a video (SDD-020 section 43, D-134).
export type ContentKind = "text" | "video";

// Why a destination cannot take the content. `takes`: its own list (the `accepts` of a connection, the
// `publishes` of a network) does not hold the kind. `other_family`: the service sends a post only to a blog
// destination and a video only to a network (SDD-020 section 43), whatever the list says.
export type Blocked = "takes" | "other_family";

// One place a target can go. `takes` is what the server says the destination accepts; `usable` is whether this
// content can go there. The page states the service's answer and adds no rule of its own: the family follows the
// refusal of the service at scheduling, so the page never offers a destination the service refuses.
export interface Destination {
	id: string;
	label: string;
	network: string;
	takes: readonly string[];
	usable: boolean;
	blocked?: Blocked;
}

function destination(
	id: string,
	label: string,
	network: string,
	takes: readonly string[],
	sameFamily: boolean,
	kind: ContentKind,
): Destination {
	// What the destination's own list says comes first: it is the more telling reason.
	if (!takes.includes(kind)) {
		return { id, label, network, takes, usable: false, blocked: "takes" };
	}
	if (!sameFamily) {
		return {
			id,
			label,
			network,
			takes,
			usable: false,
			blocked: "other_family",
		};
	}
	return { id, label, network, takes, usable: true };
}

// Every destination the person has, the ones that take the content first. A destination that cannot take it stays
// in the list, disabled, so the person sees why instead of wondering where it went.
export function destinationsFor(
	kind: ContentKind,
	accounts: readonly SocialAccount[],
	networks: readonly SocialNetwork[],
	blogs: readonly Connection[],
): Destination[] {
	const fromAccounts = accounts
		.filter((a) => a.state === "ready")
		.map((a) =>
			destination(
				a.connection_id,
				a.label,
				a.network,
				networks.find((n) => n.network === a.network)?.publishes ?? [],
				kind === "video",
				kind,
			),
		);
	const fromBlogs = blogs.map((b) =>
		destination(b.id, b.label, b.kind, b.accepts, kind === "text", kind),
	);
	const all = [...fromBlogs, ...fromAccounts];
	return [...all.filter((d) => d.usable), ...all.filter((d) => !d.usable)];
}
