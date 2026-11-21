import {
	type FormEvent,
	type ReactNode,
	useEffect,
	useId,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../editor/api/client";
import { HttpTransport } from "../editor/api/http";

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// Imports a sound file into a project, the way the fonts picker takes a path on the machine of the service. The
// file is ingested by the service and appears in the list when it is ready, so the person can read the list
// again from here.
export function SoundImport({ onImported }: { onImported(): void }): ReactNode {
	const t = useT();
	const id = useId();
	const [projects, setProjects] = useState<{ id: string; name: string }[]>([]);
	const [project, setProject] = useState("");
	const [path, setPath] = useState("");
	const [failure, setFailure] = useState<Failure | null>(null);
	const [said, setSaid] = useState<string | null>(null);
	const [busy, setBusy] = useState(false);

	useEffect(() => {
		let live = true;
		void api()
			.listProjects()
			.then((list) => {
				if (!live) return;
				setProjects(list.projects.map((p) => ({ id: p.id, name: p.name })));
			})
			.catch((error: unknown) => {
				if (live) setFailure(failureOf(error));
			});
		return () => {
			live = false;
		};
	}, []);

	async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setFailure(null);
		setSaid(null);
		setBusy(true);
		try {
			const result = await api().importMedia(project, [path.trim()]);
			const entry = result.assets[0];
			if (entry?.error !== undefined) {
				setFailure({
					code: entry.error.code,
					message: entry.error.message,
					hint: entry.error.hint,
				});
				return;
			}
			setSaid(t("ui.assets.sound.import_started"));
			setPath("");
			onImported();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<form
			onSubmit={(event) => void submit(event)}
			className="flex flex-wrap items-end gap-3 rounded-xl border bg-card p-4"
			aria-labelledby={`${id}-title`}
			data-testid="sound-import"
		>
			<h2 id={`${id}-title`} className="w-full text-sm font-medium">
				{t("ui.assets.sound.import_title")}
			</h2>
			<div className="flex max-w-full min-w-0 flex-col gap-1">
				<Label htmlFor={`${id}-project`}>
					{t("ui.assets.sound.import_project")}
				</Label>
				<select
					id={`${id}-project`}
					className="h-9 w-full max-w-full rounded-md border bg-background px-2 text-sm"
					value={project}
					onChange={(event) => setProject(event.target.value)}
				>
					<option value="">{t("ui.assets.sound.import_choose")}</option>
					{projects.map((p) => (
						<option key={p.id} value={p.id}>
							{p.name}
						</option>
					))}
				</select>
			</div>
			<div className="flex min-w-64 flex-1 flex-col gap-1">
				<Label htmlFor={`${id}-path`}>{t("ui.assets.sound.import_path")}</Label>
				<Input
					id={`${id}-path`}
					autoComplete="off"
					value={path}
					onChange={(event) => setPath(event.target.value)}
				/>
			</div>
			<Button
				type="submit"
				disabled={busy || project === "" || path.trim() === ""}
			>
				{t("ui.assets.sound.import_submit")}
			</Button>
			<div className="w-full">
				<FailureNote failure={failure} />
				{said !== null && (
					<p role="status" className="text-sm text-muted-foreground">
						{said}
					</p>
				)}
			</div>
			{projects.length === 0 && (
				<p className="w-full text-xs text-muted-foreground">
					{t("ui.assets.sound.import_no_project")}
				</p>
			)}
		</form>
	);
}
