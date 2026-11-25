import { ArrowLeft, ArrowRight, History } from "lucide-react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useState,
} from "react";
import { createPortal } from "react-dom";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { PageHeaderAction } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { useUnsavedGuard } from "@/lib/unsaved";
import AgentPanel from "./AgentPanel";
import CaptionPanel from "./CaptionPanel";
import type { ProjectFlow, TextGetResult, WritingClient } from "./client";
import EditorState from "./EditorState";
import EndPanel, { type EndStatus } from "./EndPanel";
import FlowPath from "./FlowPath";
import { endOf, exportName, primaryAction } from "./meta";
import Panel from "./Panel";
import PostPanel from "./PostPanel";
import MarkdownPreview from "./Preview";
import RevisionsPanel from "./RevisionsPanel";
import ScriptPanel from "./ScriptPanel";

// One text (SDD-019 section 11): a plain Markdown editor with a preview, the panels of its kind, the revisions and
// the draft with a model. Saving makes a revision and says its number; the revision the person read goes back
// with the text, so a write made after someone else's is refused and the person decides. There is one primary
// action at a time: Save while the text has unsaved changes, then Review.
export default function TextEditor({
	client,
	projectId,
	itemId,
	actionsSlot,
	onChanged,
}: {
	client: WritingClient;
	projectId: string;
	itemId: string;
	// Where the page header takes the actions of the open text: Revisions and Send for review.
	actionsSlot: HTMLElement | null;
	onChanged: () => void;
}): ReactNode {
	const t = useT();
	const [loaded, setLoaded] = useState<TextGetResult | null>(null);
	const [title, setTitle] = useState("");
	const [body, setBody] = useState("");
	const [failure, setFailure] = useState<Failure | null>(null);
	const [notice, setNotice] = useState<string | null>(null);
	const [conflict, setConflict] = useState<number | null>(null);
	const [busy, setBusy] = useState(false);
	const [reviewed, setReviewed] = useState(false);
	const [flow, setFlow] = useState<ProjectFlow | null>(null);
	const [ended, setEnded] = useState<EndStatus | null>(null);

	const load = useCallback(async (): Promise<void> => {
		try {
			const got = await client.get(projectId, itemId);
			setLoaded(got);
			setTitle(got.item.title);
			setBody(got.body);
			setConflict(null);
			setFailure(null);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}, [client, projectId, itemId]);

	useEffect(() => {
		setLoaded(null);
		setNotice(null);
		setReviewed(false);
		setEnded(null);
		void load();
	}, [load]);

	// The path of the project is read again when the text changes: a save can finish a step.
	const revision = loaded?.item.revision;
	// biome-ignore lint/correctness/useExhaustiveDependencies: a new revision is the reason to read the path again
	useEffect(() => {
		let live = true;
		client
			.flow(projectId)
			.then((result) => {
				if (live) setFlow(result);
			})
			.catch(() => {
				if (live) setFlow(null);
			});
		return () => {
			live = false;
		};
	}, [client, projectId, revision]);

	const dirty =
		loaded !== null && (body !== loaded.body || title !== loaded.item.title);

	useUnsavedGuard(dirty, t("ui.unsaved.leave"));

	async function save(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		if (loaded === null) return;
		setBusy(true);
		setFailure(null);
		setNotice(null);
		try {
			const done = await client.write(projectId, itemId, {
				title: title === loaded.item.title ? undefined : title,
				text: body,
				base_revision: loaded.item.revision,
			});
			setLoaded({
				project_id: projectId,
				item: done.item,
				revision: done.revision,
				body,
			});
			setReviewed(false);
			setEnded(null);
			setNotice(
				done.changed
					? t("ui.write.editor.saved", {
							revision: String(done.revision.revision),
						})
					: t("ui.write.editor.unchanged", {
							revision: String(done.revision.revision),
						}),
			);
			onChanged();
		} catch (error) {
			const failed = failureOf(error);
			if (failed.code === "E_REVISION_CONFLICT") {
				// What the person typed stays; the number of the revision that won is read for the notice.
				try {
					const latest = await client.get(projectId, itemId);
					setConflict(latest.item.revision);
				} catch {
					setConflict(0);
				}
			} else {
				setFailure(failed);
			}
		} finally {
			setBusy(false);
		}
	}

	// A change of the text that was not made here (a revert, a draft): the editor reads it again.
	const changedElsewhere = useCallback(
		(message?: string): void => {
			void load().then(() => {
				setReviewed(false);
				setEnded(null);
				if (message !== undefined) setNotice(message);
				onChanged();
			});
		},
		[load, onChanged],
	);
	const drafted = useCallback(
		(): void => changedElsewhere(),
		[changedElsewhere],
	);

	// Review is a place on this page: the person is taken to the revisions and the checks of the saved text.
	// Copy and Export act on the saved text, the one the service stored, never on what is typed and not saved.
	function copy(): void {
		if (loaded === null) return;
		navigator.clipboard
			.writeText(loaded.body)
			.then(() => setEnded({ state: "copied" }))
			.catch(() => setEnded({ state: "failed" }));
	}

	function exportFile(): void {
		if (loaded === null) return;
		const file = exportName(loaded.item.title, loaded.item.id);
		const url = URL.createObjectURL(
			new Blob([loaded.body], { type: "text/markdown" }),
		);
		const link = document.createElement("a");
		link.href = url;
		link.download = file;
		link.click();
		URL.revokeObjectURL(url);
		setEnded({ state: "exported", file });
	}

	// Review is a view of this page: the comparison takes the place of the editor, and the checks of a post stay
	// beside it. The heading of the checks (a post) or of the revisions takes the focus once the view is drawn.
	function review(): void {
		setReviewed(true);
	}

	const isPost = loaded?.item.text_kind === "post";
	useEffect(() => {
		if (!reviewed) return;
		const target = document.getElementById(
			isPost ? "post-heading" : "revisions-heading",
		);
		target?.scrollIntoView({ block: "start" });
		target?.focus();
	}, [reviewed, isPost]);

	if (loaded === null) {
		return (
			<EditorState
				failure={failure}
				onRetry={() => {
					setFailure(null);
					void load();
				}}
			/>
		);
	}

	const kind = loaded.item.text_kind;
	const primary = primaryAction({ dirty, reviewed });

	return (
		<div
			className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_20rem] xl:items-start"
			data-testid="text-editor"
		>
			{actionsSlot === null
				? null
				: createPortal(
						reviewed ? (
							<PageHeaderAction
								type="button"
								variant="outline"
								onClick={() => setReviewed(false)}
							>
								<ArrowLeft aria-hidden="true" />
								{t("ui.write.editor.back_to_writing")}
							</PageHeaderAction>
						) : (
							<>
								<PageHeaderAction
									type="button"
									variant="outline"
									onClick={() => {
										const target = document.getElementById("revisions-heading");
										target?.scrollIntoView({ block: "start" });
										target?.focus();
									}}
								>
									<History aria-hidden="true" />
									{t("ui.write.editor.revisions")}
								</PageHeaderAction>
								<PageHeaderAction
									type="button"
									variant={primary === "review" ? "default" : "outline"}
									data-primary={primary === "review" ? "true" : undefined}
									disabled={dirty}
									onClick={review}
								>
									<ArrowRight aria-hidden="true" />
									{t("ui.write.editor.review")}
								</PageHeaderAction>
							</>
						),
						actionsSlot,
					)}
			<div className="flex min-w-0 flex-col gap-6">
				{reviewed ? (
					<section
						aria-label={t(`ui.write.review.title.${kind}`)}
						data-testid="review-view"
						className="flex flex-col gap-4"
					>
						<h2 className="text-2xl font-semibold text-text">
							{t(`ui.write.review.title.${kind}`)}
						</h2>
						<p className="text-sm text-text-muted">
							{t("ui.write.review.lead", { title })}
						</p>
						<RevisionsPanel
							client={client}
							projectId={projectId}
							itemId={itemId}
							latest={loaded.item.revision}
							onReverted={changedElsewhere}
						/>
						<dl className="grid grid-cols-3 gap-3 rounded-lg border border-border bg-surface p-4 text-sm">
							<div>
								<dt className="text-text-muted">
									{t("ui.write.review.words")}
								</dt>
								<dd className="text-lg font-semibold tabular-nums">
									{loaded.item.words}
								</dd>
							</div>
							<div>
								<dt className="text-text-muted">
									{t("ui.write.review.reading")}
								</dt>
								<dd className="text-lg font-semibold tabular-nums">
									{t("ui.write.meta.reading", {
										minutes: String(loaded.item.reading_minutes),
									})}
								</dd>
							</div>
							<div>
								<dt className="text-text-muted">
									{t("ui.write.review.revisions")}
								</dt>
								<dd className="text-lg font-semibold tabular-nums">
									{loaded.item.revision}
								</dd>
							</div>
						</dl>
					</section>
				) : null}
				<form className="flex flex-col gap-4" hidden={reviewed} onSubmit={save}>
					<div className="flex flex-col gap-2">
						<div className="flex flex-wrap items-center gap-2">
							<Badge variant="outline">{t(`ui.write.kind.${kind}`)}</Badge>
							<p
								className="text-sm text-text-muted"
								data-testid="text-meta"
								aria-live="off"
							>
								{t("ui.write.meta.words", {
									count: loaded.item.words,
								})}
								{` · ${t("ui.write.meta.reading", { minutes: String(loaded.item.reading_minutes) })}`}
								{` · ${t("ui.write.meta.revision", { revision: String(loaded.item.revision) })}`}
							</p>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="text-title">{t("ui.write.editor.title")}</Label>
							<Input
								id="text-title"
								value={title}
								maxLength={120}
								required
								onChange={(event) => setTitle(event.target.value)}
							/>
						</div>
					</div>
					<Tabs defaultValue="edit">
						<TabsList variant="line">
							<TabsTrigger value="edit">
								{t("ui.write.editor.tab_edit")}
							</TabsTrigger>
							<TabsTrigger value="preview">
								{t("ui.write.editor.tab_preview")}
							</TabsTrigger>
						</TabsList>
						<TabsContent value="edit">
							<div className="flex flex-col gap-1">
								<Label htmlFor="text-body">{t("ui.write.editor.body")}</Label>
								<Textarea
									id="text-body"
									rows={22}
									value={body}
									className="font-mono leading-7"
									onChange={(event) => setBody(event.target.value)}
								/>
							</div>
						</TabsContent>
						<TabsContent value="preview">
							<MarkdownPreview
								text={body}
								label={t("ui.write.editor.preview_label")}
								empty={t("ui.write.editor.preview_empty")}
							/>
						</TabsContent>
					</Tabs>
					<div className="flex flex-wrap items-center gap-3">
						<Button
							type="submit"
							variant={primary === "save" ? "default" : "outline"}
							data-primary={primary === "save" ? "true" : undefined}
							disabled={busy || !dirty}
						>
							{busy ? t("ui.write.editor.saving") : t("ui.write.editor.save")}
						</Button>
						{dirty && (
							<span className="text-sm text-text-muted">
								{t("ui.write.editor.unsaved")}
							</span>
						)}
					</div>
				</form>
				<FailureNote failure={failure} />
				<div
					role="status"
					aria-live="polite"
					className="text-sm"
					data-testid="save-status"
				>
					{notice}
				</div>
				{conflict !== null && (
					<div
						role="alert"
						className="flex flex-col gap-2 text-sm text-danger-text"
					>
						<p>
							{t("ui.write.editor.conflict", { revision: String(conflict) })}
						</p>
						<div>
							<Button
								type="button"
								variant="outline"
								onClick={() => void load()}
							>
								{t("ui.write.editor.reload")}
							</Button>
						</div>
					</div>
				)}
			</div>
			<aside className="flex min-w-0 flex-col gap-6">
				<Panel title={t("ui.write.reading.title")} testId="reading-card">
					<p className="text-3xl font-semibold tabular-nums text-text">
						{t("ui.write.meta.reading", {
							minutes: String(loaded.item.reading_minutes),
						})}
					</p>
					<p className="text-sm text-text-muted">
						{t("ui.write.meta.words", { count: loaded.item.words })}
					</p>
				</Panel>
				<FlowPath flow={flow} />
				{kind === "script" && (
					<ScriptPanel client={client} projectId={projectId} body={body} />
				)}
				{kind === "caption" && (
					<CaptionPanel client={client} projectId={projectId} body={body} />
				)}
				{kind === "post" && (
					<PostPanel
						client={client}
						projectId={projectId}
						itemId={itemId}
						revision={loaded.item.revision}
						dirty={dirty}
					/>
				)}
				{reviewed ? null : (
					<RevisionsPanel
						client={client}
						projectId={projectId}
						itemId={itemId}
						latest={loaded.item.revision}
						onReverted={changedElsewhere}
					/>
				)}
				<EndPanel
					kind={kind}
					end={endOf(kind, flow)}
					projectId={projectId}
					dirty={dirty}
					reviewed={reviewed}
					status={ended}
					onCopy={copy}
					onExport={exportFile}
				/>
				<AgentPanel
					client={client}
					item={loaded.item}
					flow={flow}
					onDone={drafted}
				/>
			</aside>
		</div>
	);
}
