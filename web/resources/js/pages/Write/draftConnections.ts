import type { Connection, ProviderView } from "../../editor/api/types";

// The connections the draft may use: those the service lists under a provider with the text capability, and that
// accept text. A connection to a publishing destination also accepts text, so `accepts` alone is not enough; which
// kinds are AI providers is the service's answer (`ProviderList`), not a rule of the page.
export function aiTextConnections(
	providers: ProviderView[],
	connections: Connection[],
): Connection[] {
	const ids = new Set(
		providers
			.filter((p) => p.capabilities.includes("text"))
			.flatMap((p) => p.connections),
	);
	return connections.filter((c) => ids.has(c.id) && c.accepts.includes("text"));
}
