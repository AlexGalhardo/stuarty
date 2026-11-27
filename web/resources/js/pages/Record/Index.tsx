import { Head } from "@inertiajs/react";
import { Monitor, Video } from "lucide-react";
import { type ReactNode, useEffect, useMemo, useState } from "react";

import { PageHeader } from "@/components/PageHeader";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { apiTransport } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { useUnsavedGuard } from "@/lib/unsaved";
import { createClient } from "../../editor/api/client";
import RecordPanel from "../../editor/record/RecordPanel";
import RecordTeleprompter from "../../editor/record/RecordTeleprompter";
import { formatClock } from "../../editor/record/recordFlow";
import { useRecordFlow } from "../../editor/record/useRecordFlow";
import { PolishTake, StyleControls, type TakeStyle } from "./PolishTake";

export default function Index(): ReactNode {
	const t = useT();
	const client = useMemo(() => createClient(apiTransport), []);
	const flow = useRecordFlow(client, undefined, true);
	const [style, setStyle] = useState<TakeStyle>({
		layout: "screen_only",
		corner: "bottom_right",
		background: "",
	});
	const [script, setScript] = useState("");
	const [teleprompter, setTeleprompter] = useState(false);
	const view = flow.view;
	const live = view.step === "recording" || view.step === "browser_live";
	const take = view.step === "added" ? view.recording : null;
	const setup = view.step === "pick" || view.step === "browser";
	useUnsavedGuard(live, t("ui.record.leave"));

	return (
		<div className="mx-auto flex max-w-6xl flex-col gap-5">
			<Head title={t("ui.record.title")} />
			<PageHeader
				title={t("ui.record.title")}
				description={t("ui.record.description")}
				actions={
					setup && (
						<span className="inline-flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm">
							<Monitor aria-hidden="true" className="size-4" />
							{t(
								view.step === "pick" ? "ui.record.native" : "ui.record.browser",
							)}
						</span>
					)
				}
			/>
			<div className="grid min-w-0 items-start gap-4 lg:grid-cols-[minmax(0,6fr)_minmax(0,5fr)]">
				<section
					aria-label={t("ui.record.title")}
					className="min-w-0 rounded-lg border border-border bg-surface p-4"
				>
					{view.step !== "added" ? (
						<RecordPanel flow={flow} onClose={flow.reset} studio />
					) : (
						take && (
							<PolishTake
								key={take.recording_id}
								client={client}
								take={take}
								style={style}
								setStyle={setStyle}
								onReset={flow.reset}
							/>
						)
					)}
				</section>
				{setup && (
					<div className="grid min-w-0 gap-4">
						<section className="rounded-lg border border-border bg-surface p-4">
							<StyleControls value={style} onChange={setStyle} />
						</section>
						<section className="grid gap-3 rounded-lg border border-border bg-surface p-4">
							{script !== "" ? (
								<RecordTeleprompter projectId={script} />
							) : (
								<>
									<Button
										type="button"
										variant="outline"
										aria-expanded={teleprompter}
										onClick={() => setTeleprompter(!teleprompter)}
									>
										{t("ui.editor.teleprompter.title")}
									</Button>
									{teleprompter && (
										<StandaloneScript onProject={setScript} client={client} />
									)}
								</>
							)}
						</section>
					</div>
				)}
			</div>
			<section
				aria-labelledby="record-recent"
				className="rounded-lg border border-border bg-surface p-4"
			>
				<h2 id="record-recent" className="mb-2 text-base font-semibold">
					{t("ui.record.recent")}
				</h2>
				{(flow.recent ?? []).length === 0 && (
					<p className="text-sm text-text-muted">{t("ui.record.empty")}</p>
				)}
				<ul className="divide-y divide-border">
					{(flow.recent ?? []).map((recording) => (
						<li
							key={recording.recording_id}
							className="flex flex-wrap items-center gap-3 py-2"
						>
							<Video aria-hidden="true" className="size-4" />
							<div className="min-w-0 flex-1">
								<UntrustedText>
									{recording.source.name ||
										t(`ui.record.source.${recording.source.kind}`)}
								</UntrustedText>
								<p className="text-sm tabular-nums text-text-muted">
									{formatClock(recording.duration_ms)} ·{" "}
									{t(`ui.record.state.${recording.state}`)}
								</p>
							</div>
							{recording.state === "ready" && (
								<Button
									type="button"
									variant="outline"
									size="sm"
									disabled={live || flow.busy}
									onClick={() => void flow.openTake?.(recording.recording_id)}
								>
									{t("ui.record.open_take")}
								</Button>
							)}
							{(recording.state === "ready" ||
								recording.state === "failed") && (
								<Button
									type="button"
									variant="ghost"
									size="sm"
									disabled={live || flow.busy}
									onClick={() => void flow.remove(recording.recording_id)}
								>
									{t("ui.editor.record.delete")}
								</Button>
							)}
						</li>
					))}
				</ul>
			</section>
		</div>
	);
}

function StandaloneScript({
	onProject,
	client,
}: {
	onProject(id: string): void;
	client: ReturnType<typeof createClient>;
}): ReactNode {
	const t = useT();
	const [projects, setProjects] = useState<
		Awaited<ReturnType<typeof client.listProjects>>["projects"]
	>([]);
	useEffect(() => {
		let active = true;
		void client.listProjects().then(
			(result) => {
				if (active) setProjects(result.projects);
			},
			() => undefined,
		);
		return () => {
			active = false;
		};
	}, [client]);

	return (
		<label className="grid gap-1 text-sm">
			{t("ui.record.script_project")}
			<select
				className="rounded-md border border-border bg-background p-2"
				defaultValue=""
				onChange={(event) => onProject(event.target.value)}
			>
				<option value="">{t("ui.record.select_project")}</option>
				{projects.map((project) => (
					<option key={project.id} value={project.id}>
						{project.name}
					</option>
				))}
			</select>
		</label>
	);
}
