import type {
	Connection,
	SocialAccount,
	SocialNetwork,
} from "../../editor/api/types";

// The kind of content a publication holds: a blog post is text, a video is a video (SDD-020 section 43, D-134).
export type ContentKind = "text" | "video";

// One place a target can go. `takes` is what the server says the destination accepts (the `accepts` of a
// connection, the `publishes` of a network); `usable` is whether that holds the kind of this content. The page
// compares two words the service gave and states the result: it adds no rule of its own.
export interface Destination {
	id: string;
	label: string;
	network: string;
	takes: readonly string[];
	usable: boolean;
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
		.map((a): Destination => {
			const takes =
				networks.find((n) => n.network === a.network)?.publishes ?? [];
			return {
				id: a.connection_id,
				label: a.label,
				network: a.network,
				takes,
				usable: takes.includes(kind),
			};
		});
	const fromBlogs = blogs.map(
		(b): Destination => ({
			id: b.id,
			label: b.label,
			network: b.kind,
			takes: b.accepts,
			usable: b.accepts.includes(kind),
		}),
	);
	const all = [...fromBlogs, ...fromAccounts];
	return [...all.filter((d) => d.usable), ...all.filter((d) => !d.usable)];
}
