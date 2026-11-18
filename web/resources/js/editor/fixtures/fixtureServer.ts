// A stand-in for the HTTP API, driven by scripted commits. It answers with the
// response shapes of SDD-003 section 2 and publishes the events of SDD-006
// section 3, so the client, the synchronization state machine and the timeline
// run before the real endpoints exist and in unit tests. It does not
// implement commands: a test scripts the diff of every commit, as the core
// would have produced it. Its own patcher is written independently of
// state/applyDiff.ts so the tests cross-check the two.

import type { ProjectDocument } from "@/generated/timeline";

import type { Timers } from "../api/events";
import type {
	StreamConnection,
	StreamHandlers,
	Transport,
	TransportRequest,
	TransportResponse,
} from "../api/transport";
import { TransportError } from "../api/transport";
import type {
	ApiErrorBody,
	Change,
	EditResult,
	JsonValue,
	ProjectGetResult,
	ProjectState,
} from "../api/types";

type Dict = Record<string, unknown>;

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

// A deterministic generator, so a test that fails can be replayed.
export function seeded(seed: number): () => number {
	let s = seed >>> 0;
	return () => {
		s = (s + 0x6d2b79f5) >>> 0;
		let t = s;
		t = Math.imul(t ^ (t >>> 15), t | 1);
		t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};
}

const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";

export function fakeId(prefix: string, n: number): string {
	let rest = n;
	let tail = "";
	for (let i = 0; i < 10; i++) {
		tail = ALPHABET[rest % 32] + tail;
		rest = Math.floor(rest / 32);
	}
	return `${prefix}_${tail}`;
}

export interface DocumentSpec {
	tracks?: number;
	itemsPerTrack?: number;
	// Frames per item; the items of a track are laid end to end.
	itemFrames?: number;
	gap?: number;
	projectId?: string;
	name?: string;
}

// Every field an item has once the core has filled its defaults: what a fetched
// document holds and what the object of an item.added change carries since the
// diff is written in the form of a fetched document.
export const ITEM_DEFAULTS = {
	scale: 1,
	opacity: 1,
	filter: "none",
	filter_amount: 1,
	speed: { num: 1, den: 1 },
	volume: 1,
	fit: "cover",
	contrast: 1,
	saturation: 1,
	fill: "#FFFFFF",
	stroke_color: "#000000",
} as const;

export function fullItem<T extends { id: string }>(
	item: T,
): T & typeof ITEM_DEFAULTS {
	return { ...ITEM_DEFAULTS, ...item };
}

// A document that matches SDD-001: caption tracks and visual tracks with
// clips and text, and an audio track.
export function makeDocument(spec: DocumentSpec = {}): ProjectDocument {
	const trackCount = spec.tracks ?? 3;
	const perTrack = spec.itemsPerTrack ?? 4;
	const frames = spec.itemFrames ?? 90;
	const gap = spec.gap ?? 15;
	const tracks: ProjectDocument["tracks"] = {};
	const items: ProjectDocument["items"] = {};
	const assets: ProjectDocument["assets"] = {
		[fakeId("ast", 1)]: {
			id: fakeId("ast", 1),
			hash: `sha256:${"9f".repeat(32)}`,
			kind: "video",
			name: "IMG_0421.MOV",
			frame_rate: { num: 30, den: 1 },
			frame_count: 900,
			width: 1080,
			height: 1920,
			has_audio: true,
		},
	};
	let itemNo = 0;
	for (let t = 0; t < trackCount; t++) {
		const trackId = fakeId("trk", t + 1);
		const audio = t % 3 === 2;
		tracks[trackId] = {
			id: trackId,
			kind: audio ? "audio" : "visual",
			name: audio ? `Audio ${t + 1}` : `Track ${t + 1}`,
			// Fractional-index style keys, ascending.
			order: `a${String.fromCharCode(48 + (t % 10))}`,
		};
		for (let i = 0; i < perTrack; i++) {
			itemNo++;
			const id = fakeId("itm", itemNo);
			const type = audio ? "audio" : t % 3 === 1 ? "text" : "clip";
			items[id] = fullItem({
				id,
				type,
				track: trackId,
				start: i * (frames + gap),
				duration: frames,
				...(type === "text" ? { text: `Title ${itemNo}` } : {}),
				...(type === "clip" || type === "audio"
					? { asset: fakeId("ast", 1), source_in: 0 }
					: {}),
			}) as ProjectDocument["items"][string];
		}
	}
	return {
		schema_version: 1,
		id: spec.projectId ?? "prj_7k2m9x4q1b",
		name: spec.name ?? "Launch teaser",
		settings: {
			width: 1080,
			height: 1920,
			fps: { num: 30, den: 1 },
			background: "#000000",
			loudness_target: -14,
		},
		assets,
		tracks,
		items,
	} as ProjectDocument;
}

// An independent patcher for the server side of the fixtures.
function patch(doc: Dict, change: Change): void {
	const [scope, verb] = change.kind.split(".") as [string, string];
	if (scope === "project" && "changes" in change) {
		setFields(doc, change.changes);
		return;
	}
	if (scope === "settings" && "changes" in change) {
		setFields(doc.settings as Dict, change.changes);
		return;
	}
	const key = {
		asset: "assets",
		track: "tracks",
		item: "items",
		font: "fonts",
	}[scope];
	if (key === undefined)
		throw new Error(`fixture: unknown change ${change.kind}`);
	if (doc[key] === undefined) doc[key] = {};
	const collection = doc[key] as Dict;
	if (verb === "added" && "object" in change)
		collection[change.id] = clone(change.object);
	else if (verb === "removed") delete collection[change.id];
	else if (verb === "updated" && "changes" in change) {
		setFields(collection[change.id] as Dict, change.changes);
	}
}

function setFields(
	target: Dict,
	changes: { [f: string]: [JsonValue, JsonValue] },
): void {
	for (const [field, [, after]] of Object.entries(changes)) {
		if (after === null) delete target[field];
		else target[field] = clone(after);
	}
}

export interface CommitOptions {
	actor?: string;
	description?: string;
	kind?: "edit" | "undo" | "redo";
	// "stream" (default) publishes the event; "none" holds it so a test can
	// deliver the events in a chosen order with deliver().
	publish?: "stream" | "none";
	truncated?: boolean;
}

export interface Committed {
	result: EditResult;
	event: Record<string, unknown>;
}

interface Stream {
	handlers: StreamHandlers;
	open: boolean;
}

export class FixtureServer implements Transport {
	readonly projectId: string;
	revision = 0;
	state: ProjectState = "active";
	document: ProjectDocument;
	readonly log: { revision: number; event: Record<string, unknown> }[] = [];
	readonly requests: TransportRequest[] = [];
	// A scripted answer for the next call of a given "METHOD path-prefix".
	readonly failures: { match: string; status: number; body: unknown }[] = [];
	readonly openedWith: number[] = [];
	private readonly streams = new Set<Stream>();
	// Answers for the endpoints the fixture does not model (media, jobs, lint,
	// undo), by "METHOD path-prefix".
	readonly routes = new Map<
		string,
		(request: TransportRequest) => TransportResponse
	>();
	editHandler: ((request: TransportRequest) => TransportResponse) | null = null;
	private dropNetwork = false;

	constructor(document: ProjectDocument = makeDocument()) {
		this.document = document;
		this.projectId = document.id;
	}

	// Applies the diff to the server copy, bumps the revision and publishes.
	commit(diff: Change[], options: CommitOptions = {}): Committed {
		const next = clone(this.document) as unknown as Dict;
		for (const change of diff) patch(next, change);
		this.document = next as unknown as ProjectDocument;
		const base = this.revision;
		this.revision++;
		const actor = options.actor ?? "local:ui";
		const description = options.description ?? `${diff.length} changes`;
		const event: Record<string, unknown> = {
			project_id: this.projectId,
			revision: this.revision,
			base_revision: base,
			kind: options.kind ?? "edit",
			actor,
			at: "2026-11-14T12:03:10.204Z",
			description,
			...(options.truncated
				? {
						diff_truncated: true,
						changed: diff.map((c) => ({ kind: c.kind, id: c.id })),
					}
				: { diff }),
		};
		this.log.push({ revision: this.revision, event });
		const result: EditResult = {
			project_id: this.projectId,
			revision: this.revision,
			base_revision: base,
			applied: true,
			dry_run: false,
			replayed: false,
			actor,
			created: [],
			effective: [],
			diff,
			findings: [],
			lint: { errors: 0, warnings: 0 },
			description,
		};
		if ((options.publish ?? "stream") === "stream") this.deliver(event);
		return { result, event };
	}

	// Sends an event to every open stream.
	deliver(event: Record<string, unknown>, name = "batch.committed"): void {
		for (const stream of this.streams) {
			if (stream.open) stream.handlers.onMessage(name, JSON.stringify(event));
		}
	}

	heartbeat(): void {
		for (const stream of this.streams)
			if (stream.open) stream.handlers.onMessage(null, "");
	}

	// Makes the next stream error: `closed` false is a browser retry, true is a
	// closed EventSource.
	failStreams(closed: boolean): void {
		for (const stream of this.streams) {
			if (closed) stream.open = false;
			stream.handlers.onError(closed);
		}
	}

	setNetworkDown(down: boolean): void {
		this.dropNetwork = down;
	}

	get streamCount(): number {
		return [...this.streams].filter((s) => s.open).length;
	}

	get headResult(): ProjectGetResult {
		const doc = this.document;
		return {
			project_id: this.projectId,
			name: doc.name,
			revision: this.revision,
			state: this.state,
			width: doc.settings.width ?? 1080,
			height: doc.settings.height ?? 1920,
			fps: "30",
			duration_frames: this.endFrame,
			duration: { frame: this.endFrame, seconds: this.endFrame / 30 },
			counts: {
				tracks: Object.keys(doc.tracks).length,
				items: Object.keys(doc.items).length,
				assets: Object.keys(doc.assets).length,
				caption_words: 0,
			},
			next_cursor: null,
		};
	}

	// The core computes end_frame; the fixture does the same so the plan
	// answers are plausible.
	get endFrame(): number {
		let end = 0;
		for (const item of Object.values(this.document.items)) {
			end = Math.max(end, item.start + item.duration);
		}
		return end;
	}

	async send(request: TransportRequest): Promise<TransportResponse> {
		this.requests.push(request);
		if (this.dropNetwork) throw new TransportError("network", "offline");
		const key = `${request.method} ${request.path}`;
		const failure = this.failures.findIndex((f) => key.startsWith(f.match));
		if (failure >= 0) {
			const [scripted] = this.failures.splice(failure, 1);
			if (scripted.status === 0)
				throw new TransportError("network", "scripted");
			return { status: scripted.status, body: scripted.body };
		}
		const id = encodeURIComponent(this.projectId);
		if (key.startsWith(`GET /projects/${id}?detail=document`)) {
			return {
				status: 200,
				body: clone({ ...this.headResult, document: this.document }),
			};
		}
		if (key.startsWith(`GET /projects/${id}/plan`)) {
			return {
				status: 200,
				body: {
					revision: this.revision,
					from: 0,
					to: 1,
					end_frame: this.endFrame,
				},
			};
		}
		if (key === `POST /projects/${id}/edit` && this.editHandler) {
			return this.editHandler(request);
		}
		for (const [match, handle] of this.routes) {
			if (key.startsWith(match)) return handle(request);
		}
		return {
			status: 404,
			body: errorBody("E_NOT_FOUND", `no fixture for ${key}`),
		};
	}

	openStream(path: string, handlers: StreamHandlers): StreamConnection {
		const since = Number(new URL(`http://x${path}`).searchParams.get("since"));
		this.openedWith.push(since);
		const stream: Stream = { handlers, open: true };
		this.streams.add(stream);
		queueMicrotask(() => {
			if (!stream.open) return;
			handlers.onOpen();
			// The server replays whatever was committed after `since`.
			for (const entry of this.log) {
				if (entry.revision > since)
					handlers.onMessage("batch.committed", JSON.stringify(entry.event));
			}
		});
		return {
			close: () => {
				stream.open = false;
				this.streams.delete(stream);
			},
		};
	}
}

export function errorBody(code: string, message: string): ApiErrorBody {
	return { code, message, retryable: false };
}

interface Pending {
	at: number;
	callback: () => void;
	handle: number;
}

// Fake timers for the tests: nothing runs until advance() is called.
export class ManualTimers implements Timers {
	private nowMs = 0;
	private next = 1;
	private pending: Pending[] = [];

	setTimeout(callback: () => void, ms: number): unknown {
		const handle = this.next++;
		this.pending.push({ at: this.nowMs + ms, callback, handle });
		return handle;
	}

	clearTimeout(handle: unknown): void {
		this.pending = this.pending.filter((p) => p.handle !== handle);
	}

	now(): number {
		return this.nowMs;
	}

	// Moves the clock and runs every callback that became due, in order.
	async advance(ms: number): Promise<void> {
		const target = this.nowMs + ms;
		for (;;) {
			const due = this.pending
				.filter((p) => p.at <= target)
				.sort((a, b) => a.at - b.at || a.handle - b.handle)[0];
			if (due === undefined) break;
			this.pending = this.pending.filter((p) => p !== due);
			this.nowMs = due.at;
			due.callback();
			await flush();
		}
		this.nowMs = target;
	}

	get pendingCount(): number {
		return this.pending.length;
	}
}

// Lets promises and microtasks settle.
export async function flush(times = 5): Promise<void> {
	for (let i = 0; i < times; i++) await Promise.resolve();
	await new Promise((resolve) => setTimeout(resolve, 0));
}
