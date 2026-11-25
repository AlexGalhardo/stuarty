import {
	createContext,
	type ReactNode,
	useContext,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";

import { useT } from "@/lib/i18n";
import { ApiError } from "../api/errors";
import type {
	TranscriptFillersResult,
	TranscriptViewResult,
} from "../api/types";
import { transcriptDeleteOperations } from "../commands/builders";
import { useActions, useCanEdit } from "../commands/useActions";
import {
	checkedRanges,
	extendSelection,
	initialChecked,
	type SnapSummary,
	selectedRange,
	snapSummary,
	toggled,
	type WordSelection,
} from "../inspector/panels/transcriptPanelModel";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { errorHeadline } from "../ui/errorHeadline";

// What the two halves of the edit by text screen share (final design 26, SDD-023 section 6): the transcript at the
// left and the review at the right read and change the same words, selection and suggestions. The screen draws what
// the service says and sends word indices; the core finds the media to cut.
export interface TranscriptState {
	itemId: string;
	assetId: string;
	view: TranscriptViewResult | null;
	problem: string | null;
	selection: WordSelection | null;
	selectWord(index: number, extend: boolean): void;
	selectedCount: number;
	fillers: TranscriptFillersResult | null;
	checked: ReadonlySet<number>;
	toggle(index: number): void;
	busy: boolean;
	canEdit: boolean;
	snap: boolean;
	setSnap(on: boolean): void;
	snapId: string;
	snapped: { revision: number; summary: SnapSummary } | null;
	revision: number | null;
	find(): Promise<void>;
	keepAll(): void;
	deleteSelected(): Promise<void>;
	removeChecked(): Promise<void>;
}

const Context = createContext<TranscriptState | null>(null);

export function useTranscriptState(): TranscriptState | null {
	return useContext(Context);
}

export function TranscriptProvider({
	itemId,
	children,
}: {
	itemId: string;
	children: ReactNode;
}): ReactNode {
	const state = useTranscript(itemId);
	if (state === null) return children;
	return <Context.Provider value={state}>{children}</Context.Provider>;
}

// Wraps the editor while the edit by text screen is open, so that its two halves share one state.
export function TranscriptScope({
	item,
	children,
}: {
	item: string | null;
	children: ReactNode;
}): ReactNode {
	return item === null ? (
		children
	) : (
		<TranscriptProvider itemId={item}>{children}</TranscriptProvider>
	);
}

function useTranscript(itemId: string): TranscriptState | null {
	const t = useT();
	const runtime = useRuntime();
	const actions = useActions();
	const canEdit = useCanEdit();
	const assetId = useDocumentState((s) => s.document?.items[itemId]?.asset);
	const hasAudio = useDocumentState((s) => {
		const asset =
			assetId === undefined ? undefined : s.document?.assets[assetId];
		return asset?.kind === "video" && asset.has_audio !== false;
	});
	const revision = useDocumentState((s) => s.revision);
	const [view, setView] = useState<TranscriptViewResult | null>(null);
	const [problem, setProblem] = useState<string | null>(null);
	const [selection, setSelection] = useState<WordSelection | null>(null);
	const [fillers, setFillers] = useState<TranscriptFillersResult | null>(null);
	const [checked, setChecked] = useState<ReadonlySet<number>>(new Set());
	const [busy, setBusy] = useState(false);
	// Cut the edges at the word times when off; the core snaps them to the silence beside them when on (SDD-023 3.1).
	const [snap, setSnap] = useState(true);
	const snapId = useId();
	const [snapped, setSnapped] = useState<{
		revision: number;
		summary: SnapSummary;
	} | null>(null);
	// `t` is a new function on every render: the effect below must not depend on it, or it would read the
	// transcript again on every render and clear the selection.
	const translate = useRef(t);
	translate.current = t;

	// The view is recomputed by the service after every edit, so it is read again when the revision moves.
	// biome-ignore lint/correctness/useExhaustiveDependencies: revision is the trigger
	useEffect(() => {
		if (!hasAudio || assetId === undefined) return;
		let current = true;
		runtime.client
			.getTranscript(runtime.projectId, assetId)
			.then((result) => {
				if (!current) return;
				setView(result);
				setProblem(null);
			})
			.catch((error: unknown) => {
				if (!current) return;
				setView(null);
				setProblem(
					error instanceof ApiError
						? `${errorHeadline(translate.current, error.code)} ${error.hint ?? ""}`.trim()
						: translate.current("ui.editor.transcript.failed"),
				);
			});
		setSelection(null);
		setFillers(null);
		return () => {
			current = false;
		};
	}, [runtime, assetId, hasAudio, revision]);

	if (!hasAudio || assetId === undefined) return null;
	const asset = assetId;
	const range = selectedRange(selection);

	async function runCut(ranges: { from: number; to: number }[]): Promise<void> {
		setBusy(true);
		try {
			const outcome = await actions.runOperations(
				transcriptDeleteOperations(asset, ranges, { snap }),
			);
			const summary = outcome?.ok === true ? snapSummary(outcome.result) : null;
			if (outcome?.ok === true && summary !== null) {
				setSnapped({ revision: outcome.result.revision, summary });
			}
		} finally {
			setBusy(false);
		}
	}

	return {
		itemId,
		assetId: asset,
		view,
		problem,
		selection,
		selectWord: (index, extend) =>
			setSelection((was) => extendSelection(was, index, extend)),
		selectedCount: range === null ? 0 : range.to - range.from + 1,
		fillers,
		checked,
		toggle: (index) => setChecked((was) => toggled(was, index)),
		busy,
		canEdit,
		snap,
		setSnap,
		snapId,
		snapped,
		revision,
		find: async () => {
			setBusy(true);
			try {
				const found = await runtime.client.getTranscriptFillers(
					runtime.projectId,
					asset,
				);
				setFillers(found);
				setChecked(initialChecked(found.suggestions));
				setProblem(null);
			} catch (error) {
				setProblem(
					error instanceof ApiError
						? `${errorHeadline(t, error.code)} ${error.hint ?? ""}`.trim()
						: t("ui.editor.transcript.failed"),
				);
			} finally {
				setBusy(false);
			}
		},
		keepAll: () => setFillers(null),
		deleteSelected: async () => {
			if (range !== null) await runCut([range]);
		},
		removeChecked: async () => {
			if (fillers === null) return;
			const ranges = checkedRanges(fillers.suggestions, checked);
			if (ranges.length > 0) await runCut(ranges);
		},
	};
}
