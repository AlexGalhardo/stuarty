import { useCallback, useRef, useState } from "react";

import { ApiError } from "../api/errors";
import type {
	MotionBrief,
	MotionCheck,
	MotionRange,
	MotionReplaceResult,
	RenderOffer,
} from "../api/types";
import type { EditorRuntime } from "../state/runtime";
import { originOf } from "./motionModel";

export interface ImportFields {
	path: string;
	source: string;
	engine: string;
	engineVersion: string;
	renderKey: string;
}

export type Step<T> =
	| { status: "idle" }
	| { status: "loading" }
	| { status: "done"; value: T }
	| { status: "failed"; error: ApiError };

const IDLE = { status: "idle" } as const;

// An asset that the import answered before it was ready is read again this often, for at most this long.
const READY_POLL_MS = 500;
const READY_POLL_TIMES = 60;

function failed(error: unknown): { status: "failed"; error: ApiError } {
	return {
		status: "failed",
		error:
			error instanceof ApiError
				? error
				: new ApiError(0, { code: "E_NETWORK", message: String(error) }),
	};
}

export interface Imported {
	check: MotionCheck;
	offers: RenderOffer[];
	renderKey: string;
}

export interface MotionHandOff {
	brief: Step<MotionBrief>;
	imported: Step<Imported>;
	replaced: Step<MotionReplaceResult>;
	showBrief(range: MotionRange): Promise<void>;
	importAndCheck(fields: ImportFields, range: MotionRange): Promise<void>;
	replace(): Promise<void>;
}

// The hand-off from an external motion engine (SDD-011 section 15). Every answer is the service's: the brief says
// what a render must fit, the check says whether the imported file does, the replacement is one batch the core
// applies. Nothing here runs an engine or decides whether a file fits.
export function useMotionHandOff(runtime: EditorRuntime): MotionHandOff {
	const [brief, setBrief] = useState<Step<MotionBrief>>(IDLE);
	const [imported, setImported] = useState<Step<Imported>>(IDLE);
	const [replaced, setReplaced] = useState<Step<MotionReplaceResult>>(IDLE);
	const last = useRef<{ assetId: string; renderKey: string } | null>(null);
	const { client, projectId } = runtime;

	const showBrief = useCallback(
		async (range: MotionRange) => {
			setBrief({ status: "loading" });
			try {
				const answer = await client.getMotionBrief(projectId, range);
				// A range with no caption has none listed, which the service writes as null.
				setBrief({
					status: "done",
					value: { ...answer, captions: answer.captions ?? [] },
				});
			} catch (error) {
				setBrief(failed(error));
			}
		},
		[client, projectId],
	);

	const waitReady = useCallback(
		async (assetId: string): Promise<void> => {
			for (let i = 0; i < READY_POLL_TIMES; i++) {
				const list = await client.listMedia(projectId);
				const entry = list.assets.find((a) => a.asset_id === assetId);
				if (entry?.state === "ready") return;
				if (entry?.state === "failed") {
					throw new ApiError(422, {
						code: "E_INVALID_ARGUMENT",
						message: "the file could not be read",
					});
				}
				await new Promise((resolve) => setTimeout(resolve, READY_POLL_MS));
			}
		},
		[client, projectId],
	);

	const importAndCheck = useCallback(
		async (fields: ImportFields, range: MotionRange) => {
			setImported({ status: "loading" });
			setReplaced(IDLE);
			try {
				const origin = originOf(fields);
				const result = await client.importRendered(
					projectId,
					fields.path.trim(),
					origin,
				);
				const entry = result.assets[0];
				if (entry?.error !== undefined) {
					throw new ApiError(422, entry.error);
				}
				const assetId = entry?.asset_id;
				if (assetId === undefined) {
					throw new ApiError(422, {
						code: "E_INVALID_ARGUMENT",
						message: "nothing was imported",
					});
				}
				if (entry?.state !== "ready") await waitReady(assetId);
				const answer = await client.checkMotion(projectId, assetId, range);
				// A file that fits has no mismatches, which the service writes as null.
				const check = { ...answer, mismatches: answer.mismatches ?? [] };
				last.current = { assetId, renderKey: origin.render_key ?? "" };
				setImported({
					status: "done",
					value: {
						check,
						offers: entry?.replaces ?? [],
						renderKey: origin.render_key ?? "",
					},
				});
			} catch (error) {
				setImported(failed(error));
			}
		},
		[client, projectId, waitReady],
	);

	const replace = useCallback(async () => {
		const target = last.current;
		if (target === null) return;
		setReplaced({ status: "loading" });
		try {
			setReplaced({
				status: "done",
				value: await client.replaceMotion(
					projectId,
					target.assetId,
					target.renderKey,
				),
			});
		} catch (error) {
			setReplaced(failed(error));
		}
	}, [client, projectId]);

	return { brief, imported, replaced, showBrief, importAndCheck, replace };
}
