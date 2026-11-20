// What the server answers about a processed sound (SDD-004 section 7.1). With
// the bitmaps this is the only place outside the client that calls fetch (rule
// R6): the preview module only asks and waits.
export type ProbeResult =
	| { kind: "ready"; window: [number, number] | null }
	| { kind: "processing" }
	| { kind: "failed" };

export type Probe = (url: string) => Promise<ProbeResult>;

// One request that asks for the first byte: it starts the render when the file
// does not exist, and tells whether it is ready (200 or 206) or not (202).
export const fetchProbe: Probe = async (url) => {
	try {
		const response = await fetch(url, {
			credentials: "same-origin",
			headers: { Range: "bytes=0-0" },
		});
		if (response.status === 202) return { kind: "processing" };
		if (response.status !== 200 && response.status !== 206) {
			return { kind: "failed" };
		}
		const header = response.headers.get("X-Stuarty-Window");
		const match = header === null ? null : /^(\d+)-(\d+)$/.exec(header);
		const window: [number, number] | null =
			match === null ? null : [Number(match[1]), Number(match[2])];
		return { kind: "ready", window };
	} catch {
		return { kind: "failed" };
	}
};
