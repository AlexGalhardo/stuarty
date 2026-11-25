import {
	createContext,
	type ReactNode,
	useContext,
	useEffect,
	useRef,
	useState,
} from "react";

import { ApiError } from "../api/errors";
import type { CutsMediaFile, CutsProposal, ToolOffer } from "../api/types";
import { useCanEdit } from "../commands/useActions";
import {
	faceOffer,
	isFaceUnavailable,
	timelineTitle,
} from "../inspector/panels/verticalCutsModel";
import { useStore } from "../state/createStore";
import { useDocumentState, useRuntime } from "../state/EditorContext";

const POLL_AFTER_MS = 5000;
const POLL_EVERY_MS = 2000;

export type CutsPhase =
	| { name: "idle" }
	| { name: "checking" }
	| { name: "tracking"; jobId: string }
	| { name: "proposing" }
	| { name: "review"; proposals: CutsProposal[]; mediaFile: CutsMediaFile }
	| { name: "failed"; code: string; message: string; hint?: string };

// What the three parts of the vertical cuts screen share (final design 27, SDD-024 section 8): the source at the
// left, the proposals on the stage and the review at the right read and change the same phase. The face model is
// offered, never downloaded, until the person installs it; a face_track job finds the faces; the service proposes the
// cuts, and nothing is created until the person accepts one. Each accepted proposal becomes a timeline of the project
// that receives the batch of commands the service built, as it came: the browser computes no range and no framing.
export interface CutsState {
	itemId: string;
	assetId: string;
	canEdit: boolean;
	phase: CutsPhase;
	offer: ToolOffer | null;
	closeOffer(): void;
	installed(): void;
	busy: boolean;
	percent: number;
	// How many proposals were accepted on this screen.
	created: number;
	// The proposals the person has not refused, and the one the review panel is about.
	visible: CutsProposal[];
	reviewing: CutsProposal | null;
	review(n: number): void;
	find(): Promise<void>;
	accept(n: number): Promise<void>;
	refuse(n: number): void;
}

const Context = createContext<CutsState | null>(null);

export function useCutsState(): CutsState | null {
	return useContext(Context);
}

// Wraps the editor while the vertical cuts screen is open, so that its three parts share one state.
export function CutsScope({
	item,
	children,
}: {
	item: string | null;
	children: ReactNode;
}): ReactNode {
	return item === null ? (
		children
	) : (
		<CutsProvider itemId={item}>{children}</CutsProvider>
	);
}

function CutsProvider({
	itemId,
	children,
}: {
	itemId: string;
	children: ReactNode;
}): ReactNode {
	const state = useCuts(itemId);
	if (state === null) return children;
	return <Context.Provider value={state}>{children}</Context.Provider>;
}

function useCuts(itemId: string): CutsState | null {
	const runtime = useRuntime();
	const canEdit = useCanEdit();
	const assetId = useDocumentState((s) => s.document?.items[itemId]?.asset);
	const isVideo = useDocumentState((s) => {
		const asset =
			assetId === undefined ? undefined : s.document?.assets[assetId];
		return asset?.kind === "video";
	});
	const [phase, setPhase] = useState<CutsPhase>({ name: "idle" });
	const [offer, setOffer] = useState<ToolOffer | null>(null);
	const [refused, setRefused] = useState<ReadonlySet<number>>(new Set());
	const [reviewN, setReviewN] = useState<number | null>(null);
	const [creating, setCreating] = useState(false);
	const [done, setDone] = useState<ReadonlySet<number>>(new Set());
	const lastChange = useRef(0);

	const jobId = phase.name === "tracking" ? phase.jobId : null;
	const job = useStore(runtime.jobs, (s) =>
		jobId === null ? undefined : s.jobs[jobId],
	);

	async function propose(id: string): Promise<void> {
		setPhase({ name: "proposing" });
		try {
			const result = await runtime.client.getCuts(runtime.projectId, id);
			setRefused(new Set());
			setDone(new Set());
			setReviewN(result.proposals[0]?.n ?? null);
			setPhase({
				name: "review",
				proposals: result.proposals,
				mediaFile: result.media_file,
			});
		} catch (error) {
			fail(error);
		}
	}

	function fail(error: unknown): void {
		if (!(error instanceof ApiError)) throw error;
		failWith(error);
	}

	// A missing face set opens its install prompt; with no set to offer (a platform the catalogue has no helper
	// for), the error itself is shown, so the screen never stays on "checking" (VV-422).
	function failWith(error: {
		code: string;
		message: string;
		hint?: string;
		details?: ApiError["details"];
	}): void {
		const failed: CutsPhase = {
			name: "failed",
			code: error.code,
			message: error.message,
			hint: error.hint,
		};
		if (!isFaceUnavailable(error)) {
			setPhase(failed);
			return;
		}
		showOffer().then(
			(shown) => {
				if (!shown) setPhase(failed);
			},
			() => setPhase(failed),
		);
	}

	async function showOffer(): Promise<boolean> {
		const status = await runtime.client.getTools();
		const found = faceOffer(status);
		if (found === undefined || found.installed) return false;
		setOffer(found);
		setPhase({ name: "idle" });
		return true;
	}

	// The track job: success asks for the proposals; failure shows its error.
	// biome-ignore lint/correctness/useExhaustiveDependencies: `propose` and `failWith` only read stable runtime members.
	useEffect(() => {
		if (job === undefined || assetId === undefined) return;
		lastChange.current = Date.now();
		if (job.state === "succeeded") {
			void propose(assetId);
		} else if (job.state === "failed" || job.state === "cancelled") {
			if (job.error === undefined) {
				setPhase({ name: "idle" });
			} else {
				failWith(job.error);
			}
		}
	}, [job, assetId]);

	// Without an event for the job for 5 s, ask for it every 2 s.
	useEffect(() => {
		if (jobId === null) return;
		lastChange.current = Date.now();
		const timer = setInterval(() => {
			if (Date.now() - lastChange.current < POLL_AFTER_MS) return;
			runtime.client
				.getJob(jobId)
				.then((fresh) => runtime.jobsWriter.upsert(fresh))
				.catch(() => undefined);
		}, POLL_EVERY_MS);
		return () => clearInterval(timer);
	}, [jobId, runtime]);

	if (!isVideo || assetId === undefined) return null;
	const asset = assetId;

	async function find(): Promise<void> {
		setPhase({ name: "checking" });
		try {
			if (await showOffer()) return;
			const started = await runtime.client.startFaceTrack(
				runtime.projectId,
				asset,
			);
			runtime.jobsWriter.upsert(started);
			setPhase({ name: "tracking", jobId: started.job_id });
		} catch (error) {
			fail(error);
		}
	}

	// The accepted proposal: a new timeline, the asset placed in it under the id the batch names (only the import
	// flow adds an asset), then the batch at the revision the placement left.
	async function accept(n: number): Promise<void> {
		if (phase.name !== "review") return;
		const proposal = phase.proposals.find((p) => p.n === n);
		if (proposal === undefined) return;
		if (creating) return;
		const { mediaFile } = phase;
		setCreating(true);
		try {
			const added = await runtime.client.addProjectItem(runtime.projectId, {
				kind: "timeline",
				title: timelineTitle(proposal),
			});
			const timelineId = added.item.timeline_id;
			if (timelineId !== undefined) {
				const placed = await runtime.client.placeCutMedia(
					runtime.projectId,
					timelineId,
					mediaFile,
				);
				const refusedFile = placed.assets.find(
					(a) => a.error !== undefined,
				)?.error;
				if (refusedFile !== undefined) throw new ApiError(422, refusedFile);
				await runtime.client.editTimeline(runtime.projectId, timelineId, {
					operations: proposal.batch,
					base_revision: placed.revision,
					idempotency_key: crypto.randomUUID(),
				});
			}
			setDone((was) => new Set(was).add(n));
		} catch (error) {
			fail(error);
		} finally {
			setCreating(false);
		}
	}

	const proposals = phase.name === "review" ? phase.proposals : [];
	const visible = proposals.filter((p) => !refused.has(p.n) && !done.has(p.n));
	return {
		itemId,
		assetId: asset,
		canEdit,
		phase,
		offer,
		closeOffer: () => setOffer(null),
		installed: () => {
			setOffer(null);
			void find();
		},
		busy:
			creating ||
			phase.name === "checking" ||
			phase.name === "tracking" ||
			phase.name === "proposing",
		created: done.size,
		percent: Math.round((job?.progress ?? 0) * 100),
		visible,
		reviewing: visible.find((p) => p.n === reviewN) ?? visible[0] ?? null,
		review: setReviewN,
		find,
		accept,
		refuse: (n) => setRefused((was) => new Set(was).add(n)),
	};
}
