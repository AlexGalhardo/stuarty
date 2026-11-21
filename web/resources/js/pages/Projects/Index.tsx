import { Head, router, usePage } from "@inertiajs/react";
import { useRef, useState } from "react";
import { flushSync } from "react-dom";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import type { ProjectsIndexProps } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { ProjectContextInput } from "../../editor/api/types";
import { BundleImport } from "./Bundles";
import ProjectCard from "./ProjectCard";
import ProjectContextForm from "./ProjectContextForm";
import { emptyContext } from "./projectSchema";
import Templates from "./Templates";
import Trash from "./Trash";

// The list and the create form of the editor (SDD-012 section 2.1). The
// controller answers from service.Service; this page only draws the result
// and sends the form (SDD-012 section 1).
export default function ProjectsIndex() {
	const t = useT();
	const locale = useLocale();
	const { projects } = usePage<ProjectsIndexProps>().props;
	// Changes when a project goes to the trash or comes back, so the trash list
	// reads again; the list above is reloaded from the controller.
	const [trashVersion, setTrashVersion] = useState(0);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [createFailure, setCreateFailure] = useState<Failure | null>(null);
	const [creating, setCreating] = useState(false);
	// With projects to show, the form waits behind a button so the list is the first thing on the page (VV-364).
	const [formOpen, setFormOpen] = useState(projects.length === 0);
	// The last project went to the trash, so the page asks for a new one. It stays open after a restore, which would
	// otherwise throw away what was typed.
	if (projects.length === 0 && !formOpen) {
		setFormOpen(true);
	}
	const formBox = useRef<HTMLDivElement>(null);

	// The button that held the focus is gone once the form is drawn, so the focus moves to its first field.
	const openForm = (): void => {
		flushSync(() => setFormOpen(true));
		formBox.current?.querySelector<HTMLElement>("input, textarea")?.focus();
	};

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

	// The JSON API, not the Inertia form: the form answers with a 409 location
	// visit, which the browser logs as a console error (VV-059). A new project has
	// no timeline, so its page opens, where a video is added with its size.
	const submit = async (values: ProjectContextInput): Promise<void> => {
		setCreateFailure(null);
		setCreating(true);
		try {
			const created = await createClient(
				new HttpTransport("/api/v1"),
			).createProject(values);
			location.assign(`/projects/${created.project_id}`);
		} catch (error) {
			setCreateFailure(failureOf(error));
			setCreating(false);
		}
	};

	return (
		<>
			<Head title={t("ui.projects.title")} />
			<div className="mx-auto flex max-w-6xl flex-col gap-10">
				<h1 className="text-2xl font-semibold">{t("ui.projects.title")}</h1>
				<section aria-labelledby="new-project">
					<h2 id="new-project" className="text-lg font-semibold">
						{t("ui.projects.new")}
					</h2>
					{formOpen ? (
						<div ref={formBox} id="new-project-form" className="mt-4">
							<ProjectContextForm
								initial={emptyContext()}
								submitLabel={t("ui.projects.create")}
								busy={creating}
								failure={createFailure}
								onEdited={() => setCreateFailure(null)}
								onSubmit={(values) => void submit(values)}
							/>
						</div>
					) : (
						<Button
							type="button"
							className="mt-4"
							aria-expanded={false}
							aria-controls="new-project-form"
							onClick={openForm}
						>
							{t("ui.projects.new_open")}
						</Button>
					)}
				</section>

				<section aria-labelledby="project-list">
					<h2 id="project-list" className="text-lg font-semibold">
						{t("ui.projects.list")}
					</h2>
					<FailureNote failure={failure} />
					{projects.length === 0 ? (
						<p className="mt-4 text-sm text-muted-foreground">
							{t("ui.projects.empty")}
						</p>
					) : (
						<ul className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
							{projects.map((project) => (
								<ProjectCard
									key={project.id}
									project={project}
									locale={locale}
									onTrash={() => void moveToTrash(project.id)}
									onChanged={() => router.reload({ only: ["projects"] })}
								/>
							))}
						</ul>
					)}
				</section>
				<Trash
					version={trashVersion}
					onRestored={() => {
						router.reload({ only: ["projects"] });
					}}
				/>
				<Templates />
				<BundleImport />
			</div>
		</>
	);
}
