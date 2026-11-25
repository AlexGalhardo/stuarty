import { Head, router, usePage } from "@inertiajs/react";
import { Compass, FolderPlus, Plus, Upload } from "lucide-react";
import { useEffect, useState } from "react";

import { EmptyState } from "@/components/EmptyState";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { PageHeader, PageHeaderAction } from "@/components/PageHeader";
import { TabBar } from "@/components/TabBar";
import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import type { ProjectsIndexProps } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import { FirstSteps } from "../../workflow/FirstSteps";
import { BundleImport } from "./Bundles";
import NewProject, { type NewProjectPick } from "./NewProject";
import NewProjectPage from "./NewProjectPage";
import ProjectCard from "./ProjectCard";
import ProjectCardText from "./ProjectCardText";
import Templates from "./Templates";
import Trash from "./Trash";

// The list of the projects and the way to a new one (SDD-012 section 2.1). The controller answers from
// service.Service; this page only draws the result and sends commands (SDD-012 section 1). A new project starts in
// a dialog: what to create first, then the briefing, and the project exists at the end of it.
export default function ProjectsIndex() {
	const t = useT();
	const locale = useLocale();
	const { projects } = usePage<ProjectsIndexProps>().props;
	// Changes when a project goes to the trash or comes back, so the trash list
	// reads again; the list above is reloaded from the controller.
	const [trashVersion, setTrashVersion] = useState(0);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [creating, setCreating] = useState(false);
	const [pick, setPick] = useState<NewProjectPick | null>(null);
	const [view, setView] = useState<"projects" | "templates" | "trash">(
		"projects",
	);

	// The home and its empty state lead here with ?new=1: the dialog opens and the address is cleaned, so a reload
	// does not open it again.
	useEffect(() => {
		const url = new URL(window.location.href);
		if (url.searchParams.get("new") === "1") {
			setCreating(true);
			url.searchParams.delete("new");
			window.history.replaceState(window.history.state, "", url);
		}
	}, []);

	const moveToTrash = async (id: string): Promise<void> => {
		setFailure(null);
		try {
			await createClient(new HttpTransport("/api/v1")).trashProject(id);
			router.reload({ only: ["projects"] });
			setTrashVersion((was) => was + 1);
		} catch (error) {
			setFailure(failureOf(error));
		}
	};

	const newProject = (
		<PageHeaderAction type="button" onClick={() => setCreating(true)}>
			<Plus aria-hidden="true" />
			{t("ui.projects.new")}
		</PageHeaderAction>
	);
	const importTemplate = (
		<PageHeaderAction
			type="button"
			variant="outline"
			onClick={() => setView("templates")}
		>
			<Upload aria-hidden="true" />
			{t("ui.projects.import_template")}
		</PageHeaderAction>
	);

	const dialog = (
		<NewProject
			open={creating}
			onOpenChange={setCreating}
			onContinue={setPick}
		/>
	);

	return (
		<>
			<Head
				title={
					pick === null
						? t("ui.projects.title")
						: t("ui.projects.new_project.title")
				}
			/>
			{pick === null ? (
				<div className="mx-auto flex max-w-7xl flex-col gap-6">
					<PageHeader
						title={t("ui.projects.title")}
						description={t("ui.projects.lead")}
						actions={
							projects.length > 0 ? (
								<>
									{importTemplate}
									{newProject}
								</>
							) : undefined
						}
					/>
					{projects.length > 0 ? (
						<section
							aria-label={t("ui.projects.next.title")}
							className="flex flex-wrap items-center gap-3 rounded-lg border border-border bg-surface px-4 py-3"
						>
							<span
								aria-hidden="true"
								className="flex size-9 items-center justify-center rounded-md bg-accent-soft text-accent-text"
							>
								<Compass className="size-5" />
							</span>
							<div className="min-w-0 flex-1 basis-64">
								<p className="text-sm font-semibold text-accent-text">
									{t("ui.projects.next.title")}
								</p>
								<p className="text-sm text-text-muted">
									{t("ui.projects.next.description")}
								</p>
							</div>
							<Button
								type="button"
								variant="ghost"
								onClick={() => setCreating(true)}
							>
								{t("ui.projects.next.action")}
							</Button>
						</section>
					) : null}

					<TabBar
						label={t("ui.projects.views.label")}
						value={view}
						onChange={(next) => setView(next as typeof view)}
						tabs={[
							{
								value: "projects",
								label: t("ui.projects.list"),
								count: projects.length,
							},
							{ value: "templates", label: t("ui.projects.templates.title") },
							{ value: "trash", label: t("ui.projects.trash.title") },
						]}
					/>
					{view === "projects" ? (
						<section
							aria-labelledby="project-list"
							className="flex flex-col gap-4"
						>
							<h2 id="project-list" className="text-lg font-semibold">
								{t("ui.projects.list")}
							</h2>
							<FailureNote failure={failure} />
							{projects.length === 0 ? (
								<>
									<EmptyState
										level={3}
										icon={FolderPlus}
										title={t("ui.projects.empty_title")}
										description={t("ui.projects.empty")}
									>
										{newProject}
									</EmptyState>
									<FirstSteps />
								</>
							) : (
								<ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
									{projects.map((project) => {
										const Card =
											project.kind === "text" ? ProjectCardText : ProjectCard;
										return (
											<Card
												key={project.id}
												project={project}
												locale={locale}
												onTrash={() => void moveToTrash(project.id)}
												onChanged={() => router.reload({ only: ["projects"] })}
											/>
										);
									})}
								</ul>
							)}
						</section>
					) : null}
					{view === "templates" ? (
						<>
							<Templates />
							<BundleImport />
						</>
					) : null}
					{view === "trash" ? (
						<Trash
							version={trashVersion}
							onRestored={() => {
								router.reload({ only: ["projects"] });
							}}
						/>
					) : null}
				</div>
			) : (
				<NewProjectPage
					pick={pick}
					onBack={() => {
						setPick(null);
						setCreating(true);
					}}
				/>
			)}
			{dialog}
		</>
	);
}
