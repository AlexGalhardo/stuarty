import { Check, FileText as FileTextIcon, Info } from "lucide-react";
import { type FormEvent, type ReactNode, useEffect, useState } from "react";

import { DetailPanel } from "@/components/DetailPanel";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { useUnsavedGuard } from "@/lib/unsaved";
import type { LibraryClient, LibraryEntry } from "./client";
import { FileText } from "./parts";

// One prompt: a reusable text with {{variables}} (SDD-018 section 6). The preview asks the service to
// render it: the replacement of the variables is a rule of core/library, not of this page.
export default function PromptPane({
	client,
	name,
	onChanged,
}: {
	client: LibraryClient;
	name: string | null;
	onChanged: (name: string | null) => void;
}): ReactNode {
	const t = useT();
	const [entry, setEntry] = useState<LibraryEntry | null>(null);
	const [newName, setNewName] = useState("");
	const [description, setDescription] = useState("");
	const [tags, setTags] = useState("");
	const [body, setBody] = useState("");
	const [variables, setVariables] = useState<readonly string[]>([]);
	const [values, setValues] = useState<{ [variable: string]: string }>({});
	const [rendered, setRendered] = useState<string | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [saved, setSaved] = useState(false);
	const [busy, setBusy] = useState(false);
	// The fields as last read or saved: what differs from it is unsaved text.
	const [baseline, setBaseline] = useState(JSON.stringify(["", "", "", ""]));
	const current = JSON.stringify([newName, description, tags, body]);
	useUnsavedGuard(current !== baseline, t("ui.unsaved.leave"));

	useEffect(() => {
		let live = true;
		setFailure(null);
		setRendered(null);
		setValues({});
		if (name === null) {
			setEntry(null);
			setNewName("");
			setDescription("");
			setTags("");
			setBody("");
			setVariables([]);
			setBaseline(JSON.stringify(["", "", "", ""]));
			return () => {
				live = false;
			};
		}
		client
			.getPrompt(name)
			.then((result) => {
				if (!live) return;
				setEntry(result.entry);
				setDescription(result.entry.description);
				setTags((result.entry.tags ?? []).join(", "));
				setBody(result.body);
				setBaseline(
					JSON.stringify([
						"",
						result.entry.description,
						(result.entry.tags ?? []).join(", "),
						result.body,
					]),
				);
				setVariables(result.variables ?? []);
				setValues(result.defaults ?? {});
			})
			.catch((error: unknown) => {
				if (live) setFailure(failureOf(error));
			});
		return () => {
			live = false;
		};
	}, [client, name]);

	async function save(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		const target = name ?? newName.trim();
		setBusy(true);
		setFailure(null);
		setSaved(false);
		try {
			const written = await client.writePrompt(target, {
				description,
				tags: tags
					.split(",")
					.map((tag) => tag.trim())
					.filter((tag) => tag !== ""),
				body,
			});
			setVariables(written.variables ?? []);
			setBaseline(current);
			setSaved(true);
			onChanged(written.entry.name);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	async function render(): Promise<void> {
		if (name === null) return;
		setBusy(true);
		setFailure(null);
		try {
			const result = await client.renderPrompt(name, values);
			setRendered(result.rendered);
		} catch (error) {
			setRendered(null);
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<div
			className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(16rem,20rem)]"
			data-testid="prompt-pane"
		>
			<DetailPanel
				title={
					<span className="break-all">
						{entry?.name ?? t("ui.library.new.prompt")}
					</span>
				}
				actions={
					<Button type="submit" form="prompt-form" disabled={busy}>
						<Check aria-hidden="true" />
						{t("ui.library.action.save")}
					</Button>
				}
			>
				<form id="prompt-form" className="flex flex-col gap-4" onSubmit={save}>
					{name === null && (
						<div className="flex flex-col gap-1">
							<Label htmlFor="prompt-name">{t("ui.library.field.name")}</Label>
							<Input
								id="prompt-name"
								value={newName}
								maxLength={64}
								required
								onChange={(e) => setNewName(e.target.value)}
							/>
							<p className="text-sm text-text-muted">
								{t("ui.library.field.name_hint")}
							</p>
						</div>
					)}
					<div className="flex flex-col gap-1">
						<Label htmlFor="prompt-description">
							{t("ui.library.field.description")}
						</Label>
						<Input
							id="prompt-description"
							value={description}
							maxLength={200}
							required
							onChange={(e) => setDescription(e.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor="prompt-body">{t("ui.library.field.body")}</Label>
						<Textarea
							id="prompt-body"
							rows={10}
							value={body}
							required
							className="font-mono"
							onChange={(e) => setBody(e.target.value)}
						/>
						<p className="flex items-center gap-1.5 text-sm text-text-muted">
							<Info aria-hidden="true" className="size-4 shrink-0" />
							{t("ui.library.prompt.variables_hint")}
						</p>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor="prompt-tags">{t("ui.library.field.tags")}</Label>
						<Input
							id="prompt-tags"
							value={tags}
							onChange={(e) => setTags(e.target.value)}
						/>
					</div>
					{saved && (
						<p role="status" className="text-sm">
							{t("ui.library.saved")}
						</p>
					)}
					<FailureNote failure={failure} />
				</form>
			</DetailPanel>
			<DetailPanel title={t("ui.library.prompt.use_title")}>
				{name === null ? (
					<p className="text-sm text-text-muted">
						{t("ui.library.prompt.save_first")}
					</p>
				) : (
					<div className="flex flex-col gap-3">
						{variables.length === 0 && (
							<p className="text-sm text-text-muted">
								{t("ui.library.prompt.no_variables")}
							</p>
						)}
						{variables.map((variable) => (
							<div key={variable} className="flex flex-col gap-1">
								<Label htmlFor={`value-${variable}`}>
									{t("ui.library.prompt.value", { name: variable })}
								</Label>
								<Input
									id={`value-${variable}`}
									value={values[variable] ?? ""}
									onChange={(e) =>
										setValues((was) => ({
											...was,
											[variable]: e.target.value,
										}))
									}
								/>
							</div>
						))}
						<Button type="button" disabled={busy} onClick={() => void render()}>
							<FileTextIcon aria-hidden="true" />
							{t("ui.library.prompt.render")}
						</Button>
						{rendered !== null && (
							<FileText
								label={t("ui.library.prompt.rendered")}
								text={rendered}
							/>
						)}
					</div>
				)}
			</DetailPanel>
		</div>
	);
}
