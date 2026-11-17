import { useEffect, useState } from "react";

import type { ApiClient } from "../api/client";
import { useRuntime } from "../state/EditorContext";

// The names the service lists, read once per client. A control of a command
// that a build does not expose (the BETA item commands in an ALPHA build) is
// drawn only when its name is here, so the UI follows the live registry and
// never a build constant (SDD-012 section 9.2).
const cache = new WeakMap<ApiClient, Promise<ReadonlySet<string>>>();

function load(client: ApiClient): Promise<ReadonlySet<string>> {
	let names = cache.get(client);
	if (names === undefined) {
		names = client.getCommands().then(
			(result) => new Set((result.commands ?? []).map((c) => c.name)),
			() => {
				cache.delete(client);
				return new Set<string>();
			},
		);
		cache.set(client, names);
	}
	return names;
}

// The names the registry lists; empty until the list arrives.
export function useCommandNames(): ReadonlySet<string> {
	const runtime = useRuntime();
	const [exposed, setExposed] = useState<ReadonlySet<string>>(new Set());
	useEffect(() => {
		let current = true;
		void load(runtime.client).then((set) => {
			if (current) setExposed(set);
		});
		return () => {
			current = false;
		};
	}, [runtime]);
	return exposed;
}

// Whether the registry the service sent holds every one of `names`.
export function useCommandsExposed(...names: string[]): boolean {
	const exposed = useCommandNames();
	return names.every((name) => exposed.has(name));
}
