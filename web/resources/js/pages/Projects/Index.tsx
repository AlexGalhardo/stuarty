import { Head, useForm, usePage } from "@inertiajs/react";
import type { FormEvent } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import {
	type ProjectSummary,
	type ProjectsIndexProps,
	SIZE_PRESETS,
	type SizePreset,
} from "@/types";
import Templates from "./Templates";

const NAME_MAX = 120;

// The list and the create form of the editor (SDD-012 section 2.1). The
// controller answers from service.Service; this page only draws the result
// and sends the form (SDD-012 section 1).
export default function ProjectsIndex() {
	const t = useT();
	const locale = useLocale();
	const { projects } = usePage<ProjectsIndexProps>().props;
	const form = useForm<{ name: string; preset: SizePreset }>({
		name: "",
		preset: "9:16",
	});

	const submit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		form.post("/projects", { onSuccess: () => form.reset("name") });
	};

	return (
		<>
			<Head title={t("ui.projects.title")} />
			<div className="mx-auto flex max-w-4xl flex-col gap-10">
				<section aria-labelledby="new-project">
					<h2 id="new-project" className="text-lg font-semibold">
						{t("ui.projects.new")}
					</h2>
					<form
						onSubmit={submit}
						className="mt-4 flex flex-col gap-5 rounded-xl border bg-card p-5 text-card-foreground"
					>
						<div className="flex flex-col gap-2">
							<Label htmlFor="project-name">{t("ui.projects.name")}</Label>
							<Input
								id="project-name"
								name="name"
								required
								maxLength={NAME_MAX}
								autoComplete="off"
								value={form.data.name}
								onChange={(e) => form.setData("name", e.target.value)}
								aria-invalid={form.errors.name ? true : undefined}
								aria-describedby={
									form.errors.name ? "project-name-error" : undefined
								}
							/>
							{form.errors.name && (
								<p
									id="project-name-error"
									role="alert"
									className="text-sm text-destructive"
								>
									{form.errors.name}
								</p>
							)}
						</div>
						<fieldset className="flex flex-col gap-2">
							<legend className="text-sm font-medium">
								{t("ui.projects.size")}
							</legend>
							<div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
								{SIZE_PRESETS.map((preset) => (
									<label
										key={preset}
										className="flex cursor-pointer flex-col gap-0.5 rounded-lg border p-3 text-sm has-checked:border-primary has-checked:bg-muted has-focus-visible:ring-3 has-focus-visible:ring-ring/50"
									>
										<span className="flex items-center gap-2 font-medium">
											<input
												type="radio"
												name="preset"
												value={preset}
												checked={form.data.preset === preset}
												onChange={() => form.setData("preset", preset)}
												className="accent-primary"
											/>
											{preset}
										</span>
										<span className="text-xs text-muted-foreground">
											{t(`ui.projects.presets.${preset}`)}
										</span>
									</label>
								))}
							</div>
							{form.errors.preset && (
								<p role="alert" className="text-sm text-destructive">
									{form.errors.preset}
								</p>
							)}
						</fieldset>
						<div>
							<Button type="submit" disabled={form.processing}>
								{t("ui.projects.create")}
							</Button>
						</div>
					</form>
				</section>

				<section aria-labelledby="project-list">
					<h2 id="project-list" className="text-lg font-semibold">
						{t("ui.projects.list")}
					</h2>
					{projects.length === 0 ? (
						<p className="mt-4 text-sm text-muted-foreground">
							{t("ui.projects.empty")}
						</p>
					) : (
						<ul className="mt-4 divide-y rounded-xl border bg-card text-card-foreground">
							{projects.map((project) => (
								<ProjectRow
									key={project.id}
									project={project}
									locale={locale}
								/>
							))}
						</ul>
					)}
				</section>
				<Templates />
			</div>
		</>
	);
}

function ProjectRow({
	project,
	locale,
}: {
	project: ProjectSummary;
	locale: string;
}) {
	const t = useT();
	return (
		<li className="flex items-center justify-between gap-4 p-4">
			<div className="min-w-0">
				<p className="truncate font-medium">
					<UntrustedText>{project.name}</UntrustedText>
				</p>
				<p className="text-xs text-muted-foreground">
					{t("ui.projects.summary", {
						width: project.width,
						height: project.height,
						fps: project.fps,
					})}
					{" · "}
					{t("ui.projects.updated", {
						date: formatDateTime(project.updated_at, locale),
					})}
				</p>
			</div>
			{/* The editor is another document, so this is a plain link, not an Inertia visit (SDD-012 section 2.2). */}
			<Button asChild variant="outline">
				<a href={`/editor/${project.id}`}>
					{t("ui.projects.open")}
					<span className="sr-only">
						{" "}
						<UntrustedText>{project.name}</UntrustedText>
					</span>
				</a>
			</Button>
		</li>
	);
}
