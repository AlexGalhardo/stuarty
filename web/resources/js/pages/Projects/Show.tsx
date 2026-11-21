import { Head, Link, router, usePage } from "@inertiajs/react";
import { ArrowDown, ArrowUp } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { useUnsavedGuard } from "@/lib/unsaved";
import { SIZE_PRESETS } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	BriefingQuestionsResult,
	BriefResult,
	ProjectContextInput,
	ProjectGoal,
	ProjectItem,
} from "../../editor/api/types";
import { Briefing } from "./Briefing";
import ProjectContextForm from "./ProjectContextForm";

const TITLE_MAX = 120;

interface ShowProps {
	project: {
		id: string;
		name: string;
		state: string;
		description: string;
		goal: ProjectGoal;
		complete: boolean;
	};
	items: ProjectItem[];
	brief: BriefResult;
	briefing: BriefingQuestionsResult;
	[key: string]: unknown;
}

// What the creator wrote about the project, and the way to write it: a project made before epic 56 has neither, and
// opens with a notice that leads to the same form as an edit (ProjectUpdate).
function ContextPanel({ project }: { project: ShowProps["project"] }) {
	const t = useT();
	const [open, setOpen] = useState(false);
	const [busy, setBusy] = useState(false);
	// Kept here and shown in the form: a note on the page would sit behind the dialog.
	const [failure, setFailure] = useState<Failure | null>(null);

	const save = async (values: ProjectContextInput): Promise<void> => {
		setBusy(true);
		setFailure(null);
		try {
			await api().updateProject(project.id, values);
			setOpen(false);
			router.reload();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	};

	return (
		<section aria-labelledby="context-title" className="flex flex-col gap-3">
			<h2 id="context-title" className="text-lg font-semibold">
				{t("ui.projects.context.title")}
			</h2>
			{project.complete ? (
				<div className="flex flex-col gap-2 text-sm">
					<p>
						<UntrustedText>{project.description}</UntrustedText>
					</p>
					<dl className="flex flex-col gap-2">
						{(
							[
								"specific",
								"measurable",
								"achievable",
								"relevant",
								"time_bound",
								"due",
							] as const
						).map((key) => (
							<div key={key}>
								<dt className="font-medium">
									{t(`ui.projects.context.${key}`)}
								</dt>
								<dd>
									<UntrustedText>{project.goal[key]}</UntrustedText>
								</dd>
							</div>
						))}
					</dl>
				</div>
			) : (
				<div
					role="note"
					className="rounded-lg border border-dashed p-4 text-sm"
					data-testid="complete-notice"
				>
					<p className="font-medium">{t("ui.projects.complete.title")}</p>
					<p className="mt-1 text-muted-foreground">
						{t("ui.projects.complete.body")}
					</p>
				</div>
			)}
			<div>
				<Button type="button" variant="outline" onClick={() => setOpen(true)}>
					{project.complete
						? t("ui.projects.edit.open")
						: t("ui.projects.complete.action")}
				</Button>
			</div>
			<Dialog
				open={open}
				onOpenChange={(next) => {
					setOpen(next);
					setFailure(null);
				}}
			>
				<DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
					<DialogHeader>
						<DialogTitle>
							{project.complete
								? t("ui.projects.edit.title")
								: t("ui.projects.complete.title")}
						</DialogTitle>
					</DialogHeader>
					<ProjectContextForm
						initial={{
							name: project.name,
							description: project.description,
							goal: project.goal,
						}}
						keepDue={project.goal.due}
						submitLabel={t("ui.projects.edit.save")}
						busy={busy}
						failure={failure}
						onEdited={() => setFailure(null)}
						onSubmit={(values) => void save(values)}
						onCancel={() => {
							setOpen(false);
							setFailure(null);
						}}
					/>
				</DialogContent>
			</Dialog>
		</section>
	);
}

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// One item of the production: its kind, its title, and what can be done with it. A timeline opens in the
// editor; every item can be renamed, moved in the order and sent to the trash.
function ItemRow({
	projectId,
	item,
	before,
	after,
	onChanged,
	onFailure,
}: {
	projectId: string;
	item: ProjectItem;
	// The positions of the items next to this one: where a move up and a move down go.
	before: number | undefined;
	after: number | undefined;
	onChanged(): void;
	onFailure(failure: Failure): void;
}): ReactNode {
	const t = useT();
	const [renaming, setRenaming] = useState(false);
	const [title, setTitle] = useState(item.title);

	const run = async (action: () => Promise<unknown>): Promise<void> => {
		try {
			await action();
			onChanged();
		} catch (error) {
			onFailure(failureOf(error));
		}
	};

	const submitRename = async (
		event: FormEvent<HTMLFormElement>,
	): Promise<void> => {
		event.preventDefault();
		const next = title.trim();
		setRenaming(false);
		if (next === "" || next === item.title) return;
		await run(() => api().renameProjectItem(projectId, item.id, next));
	};

	return (
		<li
			data-item={item.id}
			className="flex flex-wrap items-center gap-2 rounded-lg border bg-card p-3 text-card-foreground"
		>
			<Badge variant="secondary">
				{t(`ui.projects.overview.kind.${item.kind}`)}
			</Badge>
			<span className="min-w-0 flex-1 truncate font-medium">
				<UntrustedText>{item.title}</UntrustedText>
			</span>
			{item.kind === "text" && item.text_kind !== undefined && (
				<Badge variant="outline">{t(`ui.write.kind.${item.text_kind}`)}</Badge>
			)}
			{item.kind === "text" && (
				<Button asChild variant="outline" size="sm">
					<a
						href={`/write?project=${encodeURIComponent(projectId)}&text=${encodeURIComponent(item.id)}`}
					>
						{t("ui.projects.overview.write")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{item.title}</UntrustedText>
						</span>
					</a>
				</Button>
			)}
			{item.kind === "timeline" && item.timeline_id !== undefined && (
				<Button asChild variant="outline" size="sm">
					<a href={`/editor/${item.timeline_id}`}>
						{t("ui.projects.open")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{item.title}</UntrustedText>
						</span>
					</a>
				</Button>
			)}
			<Button
				type="button"
				variant="ghost"
				size="icon-sm"
				disabled={before === undefined}
				onClick={() =>
					void run(() => api().moveProjectItem(projectId, item.id, before ?? 0))
				}
			>
				<ArrowUp aria-hidden="true" />
				<span className="sr-only">
					{t("ui.projects.overview.move_up")}{" "}
					<UntrustedText>{item.title}</UntrustedText>
				</span>
			</Button>
			<Button
				type="button"
				variant="ghost"
				size="icon-sm"
				disabled={after === undefined}
				onClick={() =>
					void run(() => api().moveProjectItem(projectId, item.id, after ?? 0))
				}
			>
				<ArrowDown aria-hidden="true" />
				<span className="sr-only">
					{t("ui.projects.overview.move_down")}{" "}
					<UntrustedText>{item.title}</UntrustedText>
				</span>
			</Button>
			<Button
				type="button"
				variant="outline"
				size="sm"
				onClick={() => {
					setTitle(item.title);
					setRenaming(true);
				}}
			>
				{t("ui.projects.card.rename")}
				<span className="sr-only">
					{" "}
					<UntrustedText>{item.title}</UntrustedText>
				</span>
			</Button>
			<Button
				type="button"
				variant="outline"
				size="sm"
				onClick={() =>
					void run(() => api().trashProjectItem(projectId, item.id))
				}
			>
				{t("ui.projects.trash.move")}
				<span className="sr-only">
					{" "}
					<UntrustedText>{item.title}</UntrustedText>
				</span>
			</Button>

			<Dialog open={renaming} onOpenChange={setRenaming}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t("ui.projects.overview.rename_title")}</DialogTitle>
					</DialogHeader>
					<form
						className="flex flex-col gap-3"
						onSubmit={(event) => void submitRename(event)}
					>
						<div className="flex flex-col gap-1">
							<Label htmlFor={`item-title-${item.id}`}>
								{t("ui.projects.overview.item_title")}
							</Label>
							<Input
								id={`item-title-${item.id}`}
								value={title}
								maxLength={TITLE_MAX}
								required
								autoComplete="off"
								onChange={(event) => setTitle(event.target.value)}
							/>
						</div>
						<DialogFooter>
							<Button
								type="button"
								variant="outline"
								onClick={() => setRenaming(false)}
							>
								{t("ui.projects.card.cancel")}
							</Button>
							<Button type="submit">{t("ui.projects.card.save")}</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>
		</li>
	);
}

// The form that adds a text or a timeline to the production.
function AddItem({
	projectId,
	onChanged,
	onFailure,
}: {
	projectId: string;
	onChanged(): void;
	onFailure(failure: Failure): void;
}): ReactNode {
	const t = useT();
	const [kind, setKind] = useState<"text" | "timeline">("timeline");
	const [title, setTitle] = useState("");
	const [text, setText] = useState("");
	const [preset, setPreset] = useState<string>("9:16");
	const [busy, setBusy] = useState(false);

	const submit = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
		event.preventDefault();
		setBusy(true);
		try {
			await api().addProjectItem(
				projectId,
				kind === "text"
					? { kind, title: title.trim(), text }
					: { kind, title: title.trim(), preset },
			);
			setTitle("");
			setText("");
			onChanged();
		} catch (error) {
			onFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	};

	return (
		<form
			onSubmit={(event) => void submit(event)}
			className="flex flex-col gap-3 rounded-xl border bg-card p-4 text-card-foreground"
		>
			<fieldset className="flex flex-wrap gap-4">
				<legend className="text-sm font-medium">
					{t("ui.projects.overview.add")}
				</legend>
				{(["timeline", "text"] as const).map((k) => (
					<label key={k} className="flex items-center gap-2 text-sm">
						<input
							type="radio"
							name="item-kind"
							value={k}
							checked={kind === k}
							onChange={() => setKind(k)}
							className="accent-primary"
						/>
						{t(`ui.projects.overview.kind.${k}`)}
					</label>
				))}
			</fieldset>
			<div className="flex flex-col gap-1">
				<Label htmlFor="new-item-title">
					{t("ui.projects.overview.item_title")}
				</Label>
				<Input
					id="new-item-title"
					value={title}
					maxLength={TITLE_MAX}
					required
					autoComplete="off"
					onChange={(event) => setTitle(event.target.value)}
				/>
			</div>
			{kind === "text" ? (
				<div className="flex flex-col gap-1">
					<Label htmlFor="new-item-text">
						{t("ui.projects.overview.item_text")}
					</Label>
					<Textarea
						id="new-item-text"
						value={text}
						onChange={(event) => setText(event.target.value)}
					/>
				</div>
			) : (
				<div className="flex flex-col gap-1">
					<Label htmlFor="new-item-preset">{t("ui.projects.size")}</Label>
					<select
						id="new-item-preset"
						value={preset}
						onChange={(event) => setPreset(event.target.value)}
						className="h-8 w-fit rounded-lg border border-input bg-background px-2 text-sm"
					>
						{SIZE_PRESETS.map((p) => (
							<option key={p} value={p}>
								{p}
							</option>
						))}
					</select>
				</div>
			)}
			<div>
				<Button type="submit" disabled={busy}>
					{t("ui.projects.overview.add_submit")}
				</Button>
			</div>
		</form>
	);
}

// The overview of a project as a production (SDD-012 section 2.1): its brief and its items, in order. The
// page draws what the service says and sends commands; the rules of the production are the core's.
export default function ProjectShow() {
	const t = useT();
	const { project, items, brief, briefing } = usePage<ShowProps>().props;
	const [failure, setFailure] = useState<Failure | null>(null);
	const [draft, setDraft] = useState(brief.text);
	const [saving, setSaving] = useState(false);
	useUnsavedGuard(draft !== brief.text, t("ui.unsaved.leave"));

	const active = items
		.filter((i) => i.state === "active" && i.kind !== "brief")
		.sort((a, b) => a.position - b.position);
	const trashed = items.filter((i) => i.state === "trashed");

	const changed = (): void => {
		setFailure(null);
		router.reload();
	};

	const saveBrief = async (
		event: FormEvent<HTMLFormElement>,
	): Promise<void> => {
		event.preventDefault();
		setSaving(true);
		setFailure(null);
		try {
			await api().setBrief(project.id, draft);
			router.reload();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setSaving(false);
		}
	};

	return (
		<>
			<Head title={project.name} />
			<div className="mx-auto flex max-w-4xl flex-col gap-8">
				<div className="flex flex-col gap-2">
					<Link
						href="/projects"
						className="text-sm text-muted-foreground underline"
					>
						{t("ui.error.back")}
					</Link>
					<h1 className="text-2xl font-semibold">
						<UntrustedText>{project.name}</UntrustedText>
					</h1>
				</div>
				<FailureNote failure={failure} />

				<ContextPanel project={project} />

				<Briefing
					projectId={project.id}
					briefing={briefing}
					onSaved={(text) => {
						setDraft(text);
						router.reload();
					}}
				/>

				<section aria-labelledby="brief-title" className="flex flex-col gap-3">
					<h2 id="brief-title" className="text-lg font-semibold">
						{t("ui.projects.overview.brief")}
					</h2>
					<form
						onSubmit={(event) => void saveBrief(event)}
						className="flex flex-col gap-3"
					>
						<Label htmlFor="brief-text" className="sr-only">
							{t("ui.projects.overview.brief")}
						</Label>
						<Textarea
							id="brief-text"
							value={draft}
							placeholder={t("ui.projects.overview.brief_empty")}
							onChange={(event) => setDraft(event.target.value)}
						/>
						<div>
							<Button type="submit" disabled={saving || draft === brief.text}>
								{t("ui.projects.overview.brief_save")}
							</Button>
						</div>
					</form>
				</section>

				<section aria-labelledby="items-title" className="flex flex-col gap-3">
					<h2 id="items-title" className="text-lg font-semibold">
						{t("ui.projects.overview.items")}
					</h2>
					{active.length === 0 ? (
						<p className="text-sm text-muted-foreground">
							{t("ui.projects.overview.items_empty")}
						</p>
					) : (
						<ul className="flex flex-col gap-2">
							{active.map((item, i) => (
								<ItemRow
									key={item.id}
									projectId={project.id}
									item={item}
									before={active[i - 1]?.position}
									after={active[i + 1]?.position}
									onChanged={changed}
									onFailure={setFailure}
								/>
							))}
						</ul>
					)}
					<AddItem
						projectId={project.id}
						onChanged={changed}
						onFailure={setFailure}
					/>
				</section>

				{trashed.length > 0 && (
					<section aria-labelledby="item-trash" className="flex flex-col gap-3">
						<h2 id="item-trash" className="text-lg font-semibold">
							{t("ui.projects.overview.trashed")}
						</h2>
						<ul className="flex flex-col gap-2">
							{trashed.map((item) => (
								<li
									key={item.id}
									data-trashed-item={item.id}
									className="flex items-center gap-2 rounded-lg border p-3"
								>
									<Badge variant="secondary">
										{t(`ui.projects.overview.kind.${item.kind}`)}
									</Badge>
									<span className="min-w-0 flex-1 truncate">
										<UntrustedText>{item.title}</UntrustedText>
									</span>
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={() =>
											void api()
												.restoreProjectItem(project.id, item.id)
												.then(changed, (error: unknown) =>
													setFailure(failureOf(error)),
												)
										}
									>
										{t("ui.projects.trash.restore")}
										<span className="sr-only">
											{" "}
											<UntrustedText>{item.title}</UntrustedText>
										</span>
									</Button>
								</li>
							))}
						</ul>
					</section>
				)}
			</div>
		</>
	);
}
