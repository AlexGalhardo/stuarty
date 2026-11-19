import { useCallback, useEffect, useRef, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import { HttpTransport } from "../../editor/api/http";
import { createPublishApi, type PublishApi, STREAM } from "./api";
import { applyPublicationEvent } from "./publicationState";
import type { ListFilter, Publication } from "./types";

export interface PublicationsState {
	// null until the first answer
	publications: Publication[] | null;
	total: number;
	// the stream is open: the list updates by itself
	live: boolean;
	failure: Failure | null;
	api: PublishApi;
	reload(): Promise<void>;
	fail(error: unknown): void;
}

// The publications a filter selects, read once and kept live by the event stream. An event replaces the publication
// it carries at once and brings a new read of the list, since only the service knows whether a publication belongs
// to the range on screen.
export function usePublications(filter: ListFilter): PublicationsState {
	const [publications, setPublications] = useState<Publication[] | null>(null);
	const [total, setTotal] = useState(0);
	const [live, setLive] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const transport = useRef<HttpTransport | null>(null);
	if (transport.current === null) {
		transport.current = new HttpTransport("/api/v1");
	}
	const api = useRef<PublishApi | null>(null);
	if (api.current === null) {
		api.current = createPublishApi(transport.current);
	}
	const client = api.current;
	const wanted = useRef(filter);
	wanted.current = filter;
	const key = JSON.stringify(filter);

	const reload = useCallback(async (): Promise<void> => {
		try {
			const res = await client.list(wanted.current);
			setPublications(res.publications);
			setTotal(res.total);
			setFailure(null);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}, [client]);

	// biome-ignore lint/correctness/useExhaustiveDependencies: key is the filter, as text, so a new one reads again
	useEffect(() => {
		void reload();
	}, [reload, key]);

	useEffect(() => {
		const http = transport.current;
		if (http === null) return;
		const stream = http.openStream(STREAM, {
			onOpen: () => setLive(true),
			onMessage: (name, data) => {
				setPublications((was) =>
					was === null ? was : applyPublicationEvent(was, name, data).next,
				);
				if (name !== null) void reload();
			},
			onError: () => setLive(false),
		});
		return () => stream.close();
	}, [reload]);

	const fail = useCallback(
		(error: unknown) => setFailure(failureOf(error)),
		[],
	);

	return { publications, total, live, failure, api: client, reload, fail };
}
