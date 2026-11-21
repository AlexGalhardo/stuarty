import { Head, router, usePage } from "@inertiajs/react";
import { useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
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
	const [creating, setCreating] = useState(false);

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
		setFailure(null);
		setCreating(true);
		try {
			const created = await createClient(
				new HttpTransport("/api/v1"),
			).createProject(values);
			location.assign(`/projects/${created.project_id}`);
		} catch (error) {
			setFailure(failureOf(error));
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
					<div className="mt-4">
						<ProjectContextForm
							initial={emptyContext()}
							submitLabel={t("ui.projects.create")}
							busy={creating}
							onSubmit={(values) => void submit(values)}
						/>
					</div>
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
