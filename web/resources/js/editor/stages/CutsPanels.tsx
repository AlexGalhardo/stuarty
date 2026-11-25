import { Check, Info, ScanFace, ShieldCheck, Users, X } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { thumbnailUrl } from "../api/urls";
import { formatSeconds } from "../commands/units";
import Section, { ToolLine } from "../inspector/Section";
import { useStore } from "../state/createStore";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { useCutsState } from "./useCuts";

// The source video at the left of the vertical cuts screen (final design 27): its picture and length, its title,
// and what the screen promises: the faces are found on this computer, and nothing enters the timeline before the
// person accepts.
export function CutsSource(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const state = useCutsState();
	const asset = useDocumentState((s) =>
		state === null ? undefined : s.document?.assets[state.assetId],
	);
	const entry = useStore(runtime.library, (s) =>
		s.entries.find((e) => e.asset_id === state?.assetId),
	);
	if (state === null) return null;
	return (
		<section
			aria-label={t("ui.editor.cuts_stage.source")}
			className="flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-auto bg-ed-panel p-3"
		>
			<h2 className="flex items-center gap-2 text-sm font-semibold">
				<ScanFace aria-hidden="true" className="size-4 text-muted-foreground" />
				{t("ui.editor.cuts_stage.source")}
			</h2>
			<div className="relative overflow-hidden rounded-lg bg-surface-sunken">
				<img
					src={thumbnailUrl(runtime.projectId, state.assetId, 320)}
					alt=""
					className="aspect-video w-full object-cover"
				/>
				{entry?.duration_seconds != null && (
					<span className="absolute bottom-2 left-2 rounded-md bg-black/80 px-2 py-0.5 font-mono text-xs text-white tabular-nums">
						{formatSeconds(entry.duration_seconds, "timecode").slice(0, 5)}
					</span>
				)}
			</div>
			{asset?.name !== undefined && (
				<p className="text-sm font-medium">
					<UntrustedText>{asset.name}</UntrustedText>
				</p>
			)}
			<p className="flex items-start gap-2 text-xs text-muted-foreground">
				<ScanFace aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
				{t("ui.editor.cuts_stage.faces_local")}
			</p>
			<p className="flex items-start gap-2 text-xs text-muted-foreground">
				<ShieldCheck aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
				{t("ui.editor.cuts_stage.nothing_enters")}
			</p>
		</section>
	);
}

// The review at the right of the vertical cuts screen (final design 27): what the cut in review brings, the framing
// the service built and the caption it asks for, then the two buttons. The framing is the command the batch holds,
// `clip.face_frame`; the caption style is chosen on the Captions screen once the cut is a timeline.
export function CutsReview(): ReactNode {
	const t = useT();
	const state = useCutsState();
	if (state === null) return null;
	const { reviewing } = state;
	return (
		<div className="flex flex-col gap-3" data-review="cuts">
			{reviewing === null ? (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.cuts_stage.nothing_to_review")}
				</p>
			) : (
				<>
					<Section title={t("ui.editor.cuts_stage.framing")}>
						<ToolLine name="clip.face_frame" />
						<p className="flex items-center justify-between text-sm">
							<span className="text-muted-foreground">
								{t("ui.editor.cuts_stage.format")}
							</span>
							<span className="rounded-md border border-border bg-surface-sunken px-2 py-1 font-mono text-xs">
								9:16
							</span>
						</p>
						<p className="flex items-start gap-2 text-xs text-muted-foreground">
							<Info aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
							{t("ui.editor.cuts_stage.framing_note")}
						</p>
					</Section>
					<Section title={t("ui.editor.cuts_stage.caption")}>
						<ToolLine name="caption.apply_style" />
						<p className="flex items-start gap-2 text-xs text-muted-foreground">
							<Info aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
							{t("ui.editor.cuts_stage.caption_note")}
						</p>
					</Section>
					<Button
						type="button"
						className="min-h-10"
						disabled={!state.canEdit || state.busy}
						onClick={() => void state.accept(reviewing.n)}
					>
						<Check aria-hidden="true" />
						{t("ui.editor.cuts_stage.accept_this")}
					</Button>
					<Button
						type="button"
						variant="outline"
						className="min-h-10"
						onClick={() => state.refuse(reviewing.n)}
					>
						<X aria-hidden="true" />
						{t("ui.editor.cuts_stage.refuse")}
					</Button>
				</>
			)}
			<p className="flex items-start gap-2 text-xs text-muted-foreground">
				<Users aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
				{t("ui.editor.cuts_stage.only_a_person")}
			</p>
		</div>
	);
}
