import { Head } from "@inertiajs/react";
import { PenLine } from "lucide-react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useState,
} from "react";

import { EmptyState } from "@/components/EmptyState";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { confirmDiscard } from "@/lib/unsaved";
import { HttpTransport } from "../../editor/api/http";
import {
	type TextItem,
	type TextKind,
	type WriteProject,
	type WritingClient,
	writingClient,
} from "./client";
import TextEditor from "./TextEditor";
import TextList, { type Loaded } from "./TextList";

const KINDS: readonly TextKind[] = ["script", "copy", "caption", "post"];

const FIELD =
	"h-8 max-w-full rounded-lg border border-input bg-background px-2 text-sm max-md:h-11";

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
	const [projects, setProjects] = useState<readonly WriteProject[]>([]);
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

	const readProjects = useCallback(async (): Promise<void> => {
		try {
			setProjects(await client.projects());
		} catch {
			setProjects([]);
		}
	}, [client]);

	useEffect(() => {
		void readProjects();
	}, [readProjects]);

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
		setLoaded({ state: "loading" });
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

	const filtered = project !== "" || kind !== "";

	return (
		<div className="flex flex-col gap-6 p-4 md:p-6">
			<Head title={t("ui.write.title")} />
			<PageHeader
				title={t("ui.write.title")}
				description={t("ui.write.intro")}
				actions={
					<Button
						type="button"
						variant={selected === null ? "default" : "outline"}
						onClick={() => setCreating((shown) => !shown)}
					>
						{t("ui.write.new")}
					</Button>
				}
			/>
			<div className="flex flex-wrap items-end gap-3">
				<div className="flex min-w-0 max-w-full flex-col gap-1">
					<Label htmlFor="write-project">{t("ui.write.filter.project")}</Label>
					<select
						id="write-project"
						value={project}
						onChange={(event) => setProject(event.target.value)}
						className={FIELD}
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
						className={FIELD}
					>
						<option value="">{t("ui.write.filter.all_kinds")}</option>
						{KINDS.map((k) => (
							<option key={k} value={k}>
								{t(`ui.write.kind.${k}`)}
							</option>
						))}
					</select>
				</div>
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
						void readProjects();
					}}
				/>
			)}
			<div className="grid gap-6 lg:grid-cols-[20rem_minmax(0,1fr)]">
				<TextList
					loaded={loaded}
					currentId={selected?.itemId ?? null}
					filtered={filtered}
					onPick={(text) => {
						if (selected?.itemId === text.id) return;
						if (!confirmDiscard(t("ui.unsaved.leave"))) return;
						setSelected({ projectId: text.project_id, itemId: text.id });
					}}
					onRetry={() => {
						setLoaded({ state: "loading" });
						void reload();
					}}
					onMore={() => void more()}
				/>
				{/* A div, not a second main: the layout's main is the only one (SDD-012 section 18.10, the skip link). */}
				<div className="min-w-0">
					{selected === null ? (
						<EmptyState
							icon={PenLine}
							level={2}
							title={t("ui.write.pick")}
							description={t("ui.write.pick_hint")}
						/>
					) : (
						<TextEditor
							key={selected.itemId}
							client={client}
							projectId={selected.projectId}
							itemId={selected.itemId}
							onChanged={() => {
								void reload();
								void readProjects();
							}}
						/>
					)}
				</div>
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
	projects: readonly WriteProject[];
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
			if (!confirmDiscard(t("ui.unsaved.leave"))) return;
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
			className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-4"
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
						className={FIELD}
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
						className={FIELD}
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
