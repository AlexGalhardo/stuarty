import { useEffect, useState } from "react";

import type { ApiClient } from "../api/client";
import type { BrandKit } from "../api/types";
import { useRuntime } from "../state/EditorContext";

// One read of the brand kit per client for the life of the editor page: every
// colour field and the Fonts section ask for it, and a kit changes on its own
// page, not while a project is open. A refused read (an older server, a key
// limited to one project) leaves the kit empty and the fields as they were.
const reads = new WeakMap<ApiClient, Promise<BrandKit | null>>();

function read(client: ApiClient): Promise<BrandKit | null> {
	let pending = reads.get(client);
	if (pending === undefined) {
		pending = client.getBrandKit().catch(() => null);
		reads.set(client, pending);
	}
	return pending;
}

export function useBrandKit(): BrandKit | null {
	const { client } = useRuntime();
	const [kit, setKit] = useState<BrandKit | null>(null);
	useEffect(() => {
		let live = true;
		void read(client).then((value) => {
			if (live) setKit(value);
		});
		return () => {
			live = false;
		};
	}, [client]);
	return kit;
}
