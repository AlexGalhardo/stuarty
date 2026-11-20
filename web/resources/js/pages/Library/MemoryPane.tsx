import { type FormEvent, type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import { useUnsavedGuard } from "@/lib/unsaved";
import type { LibraryClient, LibraryEntry } from "./client";
import { EntryBadges, FileText } from "./parts";

// One memory note: one fact in one file (SDD-018 section 5). A person who saves a note keeps it; the
// notes an agent wrote are pending until a person keeps them.
export default function MemoryPane({
	client,
	name,
	onChanged,
}: {
	client: LibraryClient;
	name: string | null;
	onChanged: (name: string | null) => void;
}): ReactNode {
	const t = useT();
	const { privileged } = useSession();
	const [entry, setEntry] = useState<LibraryEntry | null>(null);
	const [newName, setNewName] = useState("");
	const [description, setDescription] = useState("");
	const [scope, setScope] = useState("");
	const [tags, setTags] = useState("");
	const [body, setBody] = useState("");
	const [failure, setFailure] = useState<Failure | null>(null);
	const [saved, setSaved] = useState(false);
	const [busy, setBusy] = useState(false);
	const pending = entry !== null && !entry.kept;
	// The fields as last read or saved: what differs from it is unsaved text.
	const [baseline, setBaseline] = useState(
		JSON.stringify(["", "", "", "", ""]),
	);
	const current = JSON.stringify([newName, description, scope, tags, body]);
	useUnsavedGuard(current !== baseline, t("ui.unsaved.leave"));

	useEffect(() => {
		let live = true;
		setFailure(null);
		if (name === null) {
			setEntry(null);
			setNewName("");
			setDescription("");
			setScope("");
			setTags("");
			setBody("");
			setBaseline(JSON.stringify(["", "", "", "", ""]));
			return () => {
				live = false;
			};
		}
		client
			.getMemory(name)
			.then((result) => {
				if (!live) return;
				setEntry(result.entry);
				setDescription(result.entry.description);
				setScope(result.entry.scope ?? "");
				setTags((result.entry.tags ?? []).join(", "));
				setBody(result.body);
				setBaseline(
					JSON.stringify([
						"",
						result.entry.description,
						result.entry.scope ?? "",
						(result.entry.tags ?? []).join(", "),
						result.body,
					]),
				);
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
		setBusy(true);
		setFailure(null);
		setSaved(false);
		try {
			const written = await client.writeMemory({
				name: name ?? (newName.trim() === "" ? undefined : newName.trim()),
				description,
				scope: scope.trim() === "" ? undefined : scope.trim(),
				tags: tags
					.split(",")
					.map((tag) => tag.trim())
					.filter((tag) => tag !== ""),
				body,
				// Only the button that keeps a pending note asks for it; a person who saves is kept by the service.
				keep: entry !== null && !entry.kept ? true : undefined,
			});
			setBaseline(current);
			// The note as the service kept it: after Keep the panel says it is kept, with no reload.
			setEntry(written.entry);
			setSaved(true);
			onChanged(written.entry.name);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	async function remove(): Promise<void> {
		if (name === null) return;
		setBusy(true);
		setFailure(null);
		try {
			await client.removeMemory(name);
			onChanged(null);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<div className="flex flex-col gap-3" data-testid="memory-pane">
			{entry !== null && (
				<div className="flex flex-wrap items-center gap-2">
					<h2 className="text-lg font-semibold">{entry.name}</h2>
					<EntryBadges entry={entry} />
				</div>
			)}
			{entry !== null && !entry.kept && (
				<p role="status" className="text-sm text-muted-foreground">
					{t("ui.library.memory.pending_hint")}
				</p>
			)}
			<Tabs defaultValue="edit">
				<TabsList>
					<TabsTrigger value="edit">{t("ui.library.tab.edit")}</TabsTrigger>
					<TabsTrigger value="preview">
						{t("ui.library.tab.preview")}
					</TabsTrigger>
				</TabsList>
				<TabsContent value="edit">
					<form className="flex flex-col gap-3" onSubmit={save}>
						{name === null && (
							<div className="flex flex-col gap-1">
								<Label htmlFor="memory-name">
									{t("ui.library.field.name_optional")}
								</Label>
								<Input
									id="memory-name"
									value={newName}
									maxLength={64}
									onChange={(e) => setNewName(e.target.value)}
								/>
								<p className="text-xs text-muted-foreground">
									{t("ui.library.field.name_hint")}
								</p>
							</div>
						)}
						<div className="flex flex-col gap-1">
							<Label htmlFor="memory-description">
								{t("ui.library.memory.description")}
							</Label>
							<Input
								id="memory-description"
								value={description}
								maxLength={200}
								required
								onChange={(e) => setDescription(e.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="memory-body">{t("ui.library.field.body")}</Label>
							<Textarea
								id="memory-body"
								rows={8}
								value={body}
								onChange={(e) => setBody(e.target.value)}
							/>
						</div>
						<div className="grid gap-3 sm:grid-cols-2">
							<div className="flex flex-col gap-1">
								<Label htmlFor="memory-scope">
									{t("ui.library.field.scope")}
								</Label>
								<Input
									id="memory-scope"
									value={scope}
									onChange={(e) => setScope(e.target.value)}
								/>
							</div>
							<div className="flex flex-col gap-1">
								<Label htmlFor="memory-tags">
									{t("ui.library.field.tags")}
								</Label>
								<Input
									id="memory-tags"
									value={tags}
									onChange={(e) => setTags(e.target.value)}
								/>
							</div>
						</div>
						<div className="flex flex-wrap gap-2">
							<Button
								type="submit"
								disabled={busy || (pending && !privileged)}
								aria-describedby={
									pending && !privileged ? "keep-person-only" : undefined
								}
							>
								{pending
									? t("ui.library.memory.keep")
									: t("ui.library.action.save")}
							</Button>
							{pending && <PersonOnlyNote id="keep-person-only" />}
							{name !== null && (
								<Button
									type="button"
									variant="outline"
									disabled={busy}
									onClick={() => void remove()}
								>
									{t("ui.library.action.remove")}
								</Button>
							)}
						</div>
						{saved && (
							<p role="status" className="text-sm">
								{t("ui.library.saved")}
							</p>
						)}
						<FailureNote failure={failure} />
					</form>
				</TabsContent>
				<TabsContent value="preview">
					<FileText
						label={t("ui.library.preview_label")}
						text={`${description}\n\n${body}`}
					/>
				</TabsContent>
			</Tabs>
		</div>
	);
}
