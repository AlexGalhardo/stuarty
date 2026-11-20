import { Head } from "@inertiajs/react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { ProjectListResult } from "../../editor/api/types";
import {
	type TextItem,
	type TextKind,
	type WritingClient,
	writingClient,
} from "./client";
import TextEditor from "./TextEditor";

type ProjectSummary = ProjectListResult["projects"][number];

const KINDS: readonly TextKind[] = ["script", "copy", "caption", "post"];

type Loaded =
	| { state: "loading" }
	| { state: "failed" }
	| { state: "ready"; texts: readonly TextItem[]; next?: string };

interface Selected {
	projectId: string;
	itemId: string;
}

// The section Write (SDD-019 section 11): the texts of every project in one list, filtered by project and kind, and
// the editor of the one the person picks. The page reads and writes through the API and computes nothing.
export default function WritePage(): ReactNode {
	const t = useT();
	const client: WritingClient = useMemo(
		() => writingClient(new HttpTransport("/api/v1")),
		[],
	);
	const api = useMemo(() => createClient(new HttpTransport("/api/v1")), []);
	const [projects, setProjects] = useState<readonly ProjectSummary[]>([]);
	const [project, setProject] = useState("");
	const [kind, setKind] = useState<"" | TextKind>("");
	const [loaded, setLoaded] = useState<Loaded>({ state: "loading" });
	const [selected, setSelected] = useState<Selected | null>(null);
	const [creating, setCreating] = useState(false);

	// The project page links to a text with ?project=&text=: the page opens it, and nothing else is read.
	useEffect(() => {
		const query = new URLSearchParams(window.location.search);
		const projectId = query.get("project");
		const itemId = query.get("text");
		if (projectId !== null && itemId !== null) {
			setSelected({ projectId, itemId });
		}
	}, []);

	useEffect(() => {
		let live = true;
		api
			.listProjects()
			.then((result) => {
				if (live) setProjects(result.projects);
			})
			.catch(() => {
				if (live) setProjects([]);
			});
		return () => {
			live = false;
		};
	}, [api]);

	const reload = useCallback(async (): Promise<void> => {
		try {
			const result = await client.list({
				project_id: project === "" ? undefined : project,
				text_kind: kind === "" ? undefined : kind,
			});
			setLoaded({
				state: "ready",
				texts: result.texts,
				next: result.next_cursor,
			});
		} catch {
			setLoaded({ state: "failed" });
		}
	}, [client, project, kind]);

	useEffect(() => {
		void reload();
	}, [reload]);

	async function more(): Promise<void> {
		if (loaded.state !== "ready" || loaded.next === undefined) return;
		try {
			const result = await client.list({
				project_id: project === "" ? undefined : project,
				text_kind: kind === "" ? undefined : kind,
				cursor: loaded.next,
			});
			setLoaded({
				state: "ready",
				texts: [...loaded.texts, ...result.texts],
				next: result.next_cursor,
			});
		} catch {
			setLoaded({ state: "failed" });
		}
	}

	return (
		<div className="flex flex-col gap-4 p-4">
			<Head title={t("ui.write.title")} />
			<header className="flex flex-col gap-1">
				<h1 className="text-2xl font-semibold">{t("ui.write.title")}</h1>
				<p className="text-sm text-muted-foreground">{t("ui.write.intro")}</p>
			</header>
			<div className="flex flex-wrap items-end gap-3">
				<div className="flex min-w-0 max-w-full flex-col gap-1">
					<Label htmlFor="write-project">{t("ui.write.filter.project")}</Label>
					<select
						id="write-project"
						value={project}
						onChange={(event) => setProject(event.target.value)}
						className="h-8 max-w-full rounded-lg border border-input bg-background px-2 text-sm"
					>
						<option value="">{t("ui.write.filter.all_projects")}</option>
						{projects.map((p) => (
							<option key={p.id} value={p.id}>
								{p.name}
							</option>
						))}
					</select>
				</div>
				<div className="flex min-w-0 max-w-full flex-col gap-1">
					<Label htmlFor="write-kind">{t("ui.write.filter.kind")}</Label>
					<select
						id="write-kind"
						value={kind}
						onChange={(event) => setKind(event.target.value as "" | TextKind)}
						className="h-8 max-w-full rounded-lg border border-input bg-background px-2 text-sm"
					>
						<option value="">{t("ui.write.filter.all_kinds")}</option>
						{KINDS.map((k) => (
							<option key={k} value={k}>
								{t(`ui.write.kind.${k}`)}
							</option>
						))}
					</select>
				</div>
				<Button type="button" onClick={() => setCreating((open) => !open)}>
					{t("ui.write.new")}
				</Button>
			</div>
			{creating && (
				<NewText
					client={client}
					projects={projects}
					defaultProject={project}
					onCancel={() => setCreating(false)}
					onCreated={(item) => {
						setCreating(false);
						setSelected({ projectId: item.project_id, itemId: item.id });
						void reload();
					}}
				/>
			)}
			<div className="grid gap-4 lg:grid-cols-[20rem_1fr]">
				<nav
					aria-label={t("ui.write.list.label")}
					className="flex flex-col gap-2"
				>
					{loaded.state === "loading" && (
						<p role="status" className="text-sm text-muted-foreground">
							{t("ui.write.list.loading")}
						</p>
					)}
					{loaded.state === "failed" && (
						<p role="alert" className="text-sm text-destructive">
							{t("ui.write.list.failed")}
						</p>
					)}
					{loaded.state === "ready" && loaded.texts.length === 0 && (
						<p className="text-sm text-muted-foreground">
							{t("ui.write.list.empty")}
						</p>
					)}
					{loaded.state === "ready" && (
						<ul className="flex flex-col gap-1">
							{loaded.texts.map((text) => {
								const current = selected?.itemId === text.id;
								return (
									<li key={text.id}>
										<button
											type="button"
											data-text-id={text.id}
											aria-current={current ? "true" : undefined}
											onClick={() =>
												setSelected({
													projectId: text.project_id,
													itemId: text.id,
												})
											}
											className={`flex w-full flex-col gap-1 rounded-lg border p-2 text-left text-sm hover:bg-muted ${current ? "bg-muted" : ""}`}
										>
											<span className="flex flex-wrap items-center gap-2">
												<span className="font-medium">
													<UntrustedText>{text.title}</UntrustedText>
												</span>
												<Badge variant="outline">
													{t(`ui.write.kind.${text.text_kind}`)}
												</Badge>
											</span>
											<span className="text-muted-foreground">
												{t("ui.write.list.row", {
													revision: String(text.revision),
													words: String(text.words),
												})}
											</span>
											{text.project_name !== undefined && (
												<span className="text-muted-foreground">
													<UntrustedText>
														{t("ui.write.list.in_project", {
															project: text.project_name,
														})}
													</UntrustedText>
												</span>
											)}
										</button>
									</li>
								);
							})}
						</ul>
					)}
					{loaded.state === "ready" && loaded.next !== undefined && (
						<Button type="button" variant="outline" onClick={() => void more()}>
							{t("ui.write.list.more")}
						</Button>
					)}
				</nav>
				<main className="min-w-0">
					{selected === null ? (
						<p className="text-sm text-muted-foreground">
							{t("ui.write.pick")}
						</p>
					) : (
						<TextEditor
							key={selected.itemId}
							client={client}
							projectId={selected.projectId}
							itemId={selected.itemId}
							onChanged={() => void reload()}
						/>
					)}
				</main>
			</div>
		</div>
	);
}

// The form of a new text: the project, a title and the kind. The service makes it, with its first revision.
function NewText({
	client,
	projects,
	defaultProject,
	onCancel,
	onCreated,
}: {
	client: WritingClient;
	projects: readonly ProjectSummary[];
	defaultProject: string;
	onCancel: () => void;
	onCreated: (item: TextItem) => void;
}): ReactNode {
	const t = useT();
	const [project, setProject] = useState(
		defaultProject || (projects[0]?.id ?? ""),
	);
	const [title, setTitle] = useState("");
	const [kind, setKind] = useState<TextKind>("script");
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);

	async function create(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			const made = await client.create(project, {
				title,
				text_kind: kind,
				text: "",
			});
			onCreated(made.item);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	if (projects.length === 0) {
		return <p className="text-sm">{t("ui.write.new_form.no_projects")}</p>;
	}

	return (
		<form
			className="flex flex-col gap-3 rounded-lg border p-3"
			onSubmit={create}
			aria-label={t("ui.write.new_form.title")}
		>
			<div className="grid gap-3 sm:grid-cols-3">
				<div className="flex min-w-0 max-w-full flex-col gap-1">
					<Label htmlFor="new-project">{t("ui.write.new_form.project")}</Label>
					<select
						id="new-project"
						value={project}
						onChange={(event) => setProject(event.target.value)}
						className="h-8 max-w-full rounded-lg border border-input bg-background px-2 text-sm"
					>
						{projects.map((p) => (
							<option key={p.id} value={p.id}>
								{p.name}
							</option>
						))}
					</select>
				</div>
				<div className="flex min-w-0 max-w-full flex-col gap-1">
					<Label htmlFor="new-title">{t("ui.write.new_form.name")}</Label>
					<Input
						id="new-title"
						value={title}
						maxLength={120}
						required
						onChange={(event) => setTitle(event.target.value)}
					/>
				</div>
				<div className="flex min-w-0 max-w-full flex-col gap-1">
					<Label htmlFor="new-kind">{t("ui.write.new_form.kind")}</Label>
					<select
						id="new-kind"
						value={kind}
						onChange={(event) => setKind(event.target.value as TextKind)}
						className="h-8 max-w-full rounded-lg border border-input bg-background px-2 text-sm"
					>
						{KINDS.map((k) => (
							<option key={k} value={k}>
								{t(`ui.write.kind.${k}`)}
							</option>
						))}
					</select>
				</div>
			</div>
			<FailureNote failure={failure} />
			<div className="flex gap-2">
				<Button type="submit" disabled={busy}>
					{t("ui.write.new_form.create")}
				</Button>
				<Button type="button" variant="outline" onClick={onCancel}>
					{t("ui.write.new_form.cancel")}
				</Button>
			</div>
		</form>
	);
}
