import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import CaptionPanel from "./CaptionPanel";
import type { TextGetResult, WritingClient } from "./client";
import DraftPanel from "./DraftPanel";
import MarkdownPreview from "./Preview";
import RevisionsPanel from "./RevisionsPanel";
import ScriptPanel from "./ScriptPanel";

// One text (SDD-019 section 11): a plain Markdown editor with a preview, the panels of its kind, the revisions and
// the draft with a model. Saving makes a revision and says its number; the revision the person read goes back
// with the text, so a write made after someone else's is refused and the person decides.
export default function TextEditor({
	client,
	projectId,
	itemId,
	onChanged,
}: {
	client: WritingClient;
	projectId: string;
	itemId: string;
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
		void load();
	}, [load]);

	const dirty =
		loaded !== null && (body !== loaded.body || title !== loaded.item.title);

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

	if (loaded === null) {
		return (
			<div>
				<FailureNote failure={failure} />
				{failure === null && (
					<p role="status" className="text-sm text-muted-foreground">
						{t("ui.write.editor.loading")}
					</p>
				)}
			</div>
		);
	}

	const kind = loaded.item.text_kind;

	return (
		<div className="flex flex-col gap-4" data-testid="text-editor">
			<form className="flex flex-col gap-3" onSubmit={save}>
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
				<Tabs defaultValue="edit">
					<TabsList>
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
								rows={14}
								value={body}
								className="font-mono"
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
					<Button type="submit" disabled={busy || !dirty}>
						{busy ? t("ui.write.editor.saving") : t("ui.write.editor.save")}
					</Button>
					{dirty && (
						<span className="text-sm text-muted-foreground">
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
					className="flex flex-col gap-2 text-sm text-destructive"
				>
					<p>{t("ui.write.editor.conflict", { revision: String(conflict) })}</p>
					<div>
						<Button type="button" variant="outline" onClick={() => void load()}>
							{t("ui.write.editor.reload")}
						</Button>
					</div>
				</div>
			)}
			{kind === "script" && (
				<ScriptPanel client={client} projectId={projectId} body={body} />
			)}
			{kind === "caption" && (
				<CaptionPanel client={client} projectId={projectId} body={body} />
			)}
			<DraftPanel client={client} item={loaded.item} onDone={drafted} />
			<RevisionsPanel
				client={client}
				projectId={projectId}
				itemId={itemId}
				latest={loaded.item.revision}
				onReverted={changedElsewhere}
			/>
		</div>
	);
}
