import { CircleAlert, CircleCheck, Clapperboard, Copy } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import ErrorBody from "../commands/ErrorBody";
import { useCanEdit } from "../commands/useActions";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { selectOrderedTrackIds } from "../state/selectors";
import { trackLabel } from "../ui/trackLabel";
import { fieldKey, rateLabel, sizeLabel } from "./motionModel";
import PanelFrame from "./PanelFrame";
import { type Step, useMotionHandOff } from "./useMotionHandOff";

const SELECT =
	"h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-50 dark:bg-input/30";

function Field({
	label,
	hint,
	children,
}: {
	label: string;
	hint?: string;
	children: (id: string, describedBy: string | undefined) => ReactNode;
}): ReactNode {
	const id = useId();
	const hintId = hint === undefined ? undefined : `${id}-hint`;
	return (
		<div className="flex flex-col gap-1">
			<Label htmlFor={id} className="text-xs">
				{label}
			</Label>
			{children(id, hintId)}
			{hint !== undefined && (
				<p id={hintId} className="text-xs text-muted-foreground">
					{hint}
				</p>
			)}
		</div>
	);
}

function StepHeading({
	n,
	children,
}: {
	n: number;
	children: ReactNode;
}): ReactNode {
	return (
		<h3 className="flex items-center gap-2 text-sm font-semibold">
			<span
				aria-hidden="true"
				className="flex size-5 shrink-0 items-center justify-center rounded-full bg-surface-selected text-xs"
			>
				{n}
			</span>
			{children}
		</h3>
	);
}

function Failure<T>({ step }: { step: Step<T> }): ReactNode {
	if (step.status !== "failed") return null;
	return (
		<div role="alert" className="flex flex-col gap-1 text-sm text-destructive">
			<ErrorBody error={step.error} />
		</div>
	);
}

// The hand-off from an external motion engine (D-137, SDD-011 section 15): what the animation must fit, a
// reminder that the person renders it on their own side, and the import of the file with the check of what does
// not fit. Stuarty never installs or runs the engine: the panel reads the brief, imports a file the person made
// and shows the check the service computed.
export default function MotionPanel(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const canEdit = useCanEdit();
	const flow = useMotionHandOff(runtime);
	const order = useDocumentState(selectOrderedTrackIds);
	const tracks = useDocumentState((s) => s.document?.tracks);
	const kindOf = (id: string): "audio" | "visual" =>
		tracks?.[id]?.kind === "audio" ? "audio" : "visual";
	const visual = order.filter((id) => kindOf(id) === "visual");

	const [from, setFrom] = useState("");
	const [until, setUntil] = useState("");
	const [trackId, setTrackId] = useState("");
	const [path, setPath] = useState("");
	const [source, setSource] = useState("");
	const [engine, setEngine] = useState("HyperFrames");
	const [engineVersion, setEngineVersion] = useState("");
	const [renderKey, setRenderKey] = useState("");
	const [copied, setCopied] = useState(false);
	const range = {
		...(from.trim() === "" ? {} : { from: from.trim() }),
		...(until.trim() === "" ? {} : { until: until.trim() }),
		...(trackId === "" ? {} : { trackId }),
	};
	const brief = flow.brief.status === "done" ? flow.brief.value : null;
	const done = flow.imported.status === "done" ? flow.imported.value : null;

	return (
		<PanelFrame title={t("ui.editor.panels.handoff")}>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.handoff.intro")}
			</p>

			<section
				aria-label={t("ui.editor.handoff.step_brief")}
				data-motion-step="brief"
				className="flex flex-col gap-2 border-b border-border pb-3"
			>
				<StepHeading n={1}>{t("ui.editor.handoff.step_brief")}</StepHeading>
				<div className="grid grid-cols-2 gap-2">
					<Field label={t("ui.editor.handoff.from")}>
						{(id) => (
							<Input
								id={id}
								value={from}
								placeholder={t("ui.editor.handoff.from_placeholder")}
								onChange={(event) => setFrom(event.target.value)}
							/>
						)}
					</Field>
					<Field label={t("ui.editor.handoff.until")}>
						{(id) => (
							<Input
								id={id}
								value={until}
								placeholder={t("ui.editor.handoff.until_placeholder")}
								onChange={(event) => setUntil(event.target.value)}
							/>
						)}
					</Field>
				</div>
				<Field
					label={t("ui.editor.handoff.track")}
					hint={t("ui.editor.handoff.track_hint")}
				>
					{(id, describedBy) => (
						<select
							id={id}
							aria-describedby={describedBy}
							className={SELECT}
							value={trackId}
							onChange={(event) => setTrackId(event.target.value)}
						>
							<option value="">{t("ui.editor.handoff.track_none")}</option>
							{visual.map((track) => {
								const given = tracks?.[track]?.name;
								return (
									<option key={track} value={track}>
										{given === undefined || given === ""
											? trackLabel(track, order, kindOf)
											: given}
									</option>
								);
							})}
						</select>
					)}
				</Field>
				<Button
					type="button"
					size="sm"
					variant="outline"
					disabled={flow.brief.status === "loading"}
					onClick={() => void flow.showBrief(range)}
					data-motion-action="brief"
				>
					<Clapperboard aria-hidden="true" />
					{t("ui.editor.handoff.show_brief")}
				</Button>
				<Failure step={flow.brief} />
				{brief !== null && (
					<div data-motion-brief="" className="flex flex-col gap-2">
						<dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
							<dt className="text-muted-foreground">
								{t("ui.editor.handoff.size")}
							</dt>
							<dd className="font-mono tabular-nums">
								{sizeLabel(brief.width, brief.height)}
							</dd>
							<dt className="text-muted-foreground">
								{t("ui.editor.handoff.rate")}
							</dt>
							<dd className="font-mono tabular-nums">
								{rateLabel(brief.frame_rate)}
							</dd>
							<dt className="text-muted-foreground">
								{t("ui.editor.handoff.length")}
							</dt>
							<dd className="font-mono tabular-nums">
								{t("ui.editor.handoff.length_value", {
									frames: brief.duration_frames,
									seconds: brief.duration.seconds.toFixed(2),
								})}
							</dd>
							<dt className="text-muted-foreground">
								{t("ui.editor.handoff.alpha")}
							</dt>
							<dd>
								{t(
									brief.needs_alpha
										? "ui.editor.handoff.alpha_needed"
										: "ui.editor.handoff.alpha_not_needed",
								)}
							</dd>
							<dt className="text-muted-foreground">
								{t("ui.editor.handoff.safe")}
							</dt>
							<dd className="font-mono tabular-nums">
								{t("ui.editor.handoff.safe_value", {
									left: brief.margins.left,
									right: brief.margins.right,
									top: brief.margins.top,
									bottom: brief.margins.bottom,
								})}
							</dd>
							<dt className="text-muted-foreground">
								{t("ui.editor.handoff.captions")}
							</dt>
							<dd>{brief.captions.length}</dd>
						</dl>
						<Button
							type="button"
							size="sm"
							variant="ghost"
							className="self-start"
							onClick={() => {
								void navigator.clipboard
									?.writeText(brief.text)
									.then(() => setCopied(true))
									.catch(() => setCopied(false));
							}}
						>
							<Copy aria-hidden="true" />
							{t("ui.editor.handoff.copy_brief")}
						</Button>
						{copied && (
							<p role="status" className="text-xs text-muted-foreground">
								{t("ui.editor.handoff.copied")}
							</p>
						)}
					</div>
				)}
			</section>

			<section
				aria-label={t("ui.editor.handoff.step_render")}
				data-motion-step="render"
				className="flex flex-col gap-1 border-b border-border pb-3"
			>
				<StepHeading n={2}>{t("ui.editor.handoff.step_render")}</StepHeading>
				<p className="text-sm">{t("ui.editor.handoff.render_here")}</p>
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.handoff.never_runs")}
				</p>
			</section>

			<section
				aria-label={t("ui.editor.handoff.step_import")}
				data-motion-step="import"
				className="flex flex-col gap-2"
			>
				<StepHeading n={3}>{t("ui.editor.handoff.step_import")}</StepHeading>
				<Field
					label={t("ui.editor.handoff.path")}
					hint={t("ui.editor.handoff.path_hint")}
				>
					{(id, describedBy) => (
						<Input
							id={id}
							aria-describedby={describedBy}
							value={path}
							onChange={(event) => setPath(event.target.value)}
						/>
					)}
				</Field>
				<Field label={t("ui.editor.handoff.source")}>
					{(id) => (
						<Input
							id={id}
							value={source}
							onChange={(event) => setSource(event.target.value)}
						/>
					)}
				</Field>
				<div className="grid grid-cols-2 gap-2">
					<Field label={t("ui.editor.handoff.engine")}>
						{(id) => (
							<Input
								id={id}
								value={engine}
								onChange={(event) => setEngine(event.target.value)}
							/>
						)}
					</Field>
					<Field label={t("ui.editor.handoff.engine_version")}>
						{(id) => (
							<Input
								id={id}
								value={engineVersion}
								onChange={(event) => setEngineVersion(event.target.value)}
							/>
						)}
					</Field>
				</div>
				<Field
					label={t("ui.editor.handoff.render_key")}
					hint={t("ui.editor.handoff.render_key_hint")}
				>
					{(id, describedBy) => (
						<Input
							id={id}
							aria-describedby={describedBy}
							value={renderKey}
							onChange={(event) => setRenderKey(event.target.value)}
						/>
					)}
				</Field>
				<Button
					type="button"
					size="sm"
					disabled={
						!canEdit ||
						path.trim() === "" ||
						source.trim() === "" ||
						flow.imported.status === "loading"
					}
					onClick={() =>
						void flow.importAndCheck(
							{ path, source, engine, engineVersion, renderKey },
							range,
						)
					}
					data-motion-action="import"
				>
					{t(
						flow.imported.status === "loading"
							? "ui.editor.handoff.importing"
							: "ui.editor.handoff.import",
					)}
				</Button>
				<Failure step={flow.imported} />
				{done !== null && (
					<div data-motion-check="" className="flex flex-col gap-2">
						<p
							role="status"
							data-motion-verdict={done.check.ok ? "fits" : "mismatch"}
							className="flex items-center gap-2 text-sm font-medium"
						>
							{done.check.ok ? (
								<CircleCheck
									aria-hidden="true"
									className="size-4 text-success"
								/>
							) : (
								<CircleAlert
									aria-hidden="true"
									className="size-4 text-destructive"
								/>
							)}
							<span>
								<UntrustedText>{done.check.asset_name}</UntrustedText>
								{": "}
								{t(
									done.check.ok
										? "ui.editor.handoff.fits"
										: "ui.editor.handoff.does_not_fit",
								)}
							</span>
						</p>
						{done.check.mismatches.length > 0 && (
							<ul className="flex flex-col gap-2">
								{done.check.mismatches.map((m) => {
									const key = fieldKey(m);
									return (
										<li
											key={m.field}
											data-mismatch={m.field}
											className="rounded-md border border-border p-2 text-sm"
										>
											<p className="font-medium">
												{key === null ? m.field : t(key)}
											</p>
											<p className="text-xs">
												{t("ui.editor.handoff.expected")}{" "}
												<span className="font-mono">{m.expected}</span>
												{" · "}
												{t("ui.editor.handoff.found")}{" "}
												<span className="font-mono">{m.found}</span>
											</p>
											<p className="text-xs text-muted-foreground">
												<UntrustedText>{m.hint}</UntrustedText>
											</p>
										</li>
									);
								})}
							</ul>
						)}
						{done.offers.length > 0 && (
							<div
								data-motion-offer=""
								className="flex flex-col gap-1 rounded-md bg-surface-sunken p-2"
							>
								<p className="text-sm">
									{t("ui.editor.handoff.offer", {
										count: done.offers.reduce(
											(sum, offer) => sum + offer.item_ids.length,
											0,
										),
									})}
								</p>
								<Button
									type="button"
									size="sm"
									variant="outline"
									className="self-start"
									disabled={!canEdit || flow.replaced.status === "loading"}
									onClick={() => void flow.replace()}
									data-motion-action="replace"
								>
									{t("ui.editor.handoff.replace")}
								</Button>
								<Failure step={flow.replaced} />
								{flow.replaced.status === "done" && (
									<p role="status" className="text-xs">
										{t("ui.editor.handoff.replaced", {
											count: flow.replaced.value.replaced.length,
										})}
									</p>
								)}
							</div>
						)}
					</div>
				)}
			</section>
		</PanelFrame>
	);
}
