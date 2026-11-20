import { type FormEvent, type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import type {
	LibraryClient,
	LibraryEntry,
	LibraryFinding,
	SkillFileView,
} from "./client";
import { EntryBadges, FileText, Findings } from "./parts";

// One skill (SDD-018 section 7): its origin, licence and full text, the findings of the lint and the review
// state. A skill that came from outside is not offered to an agent until a person has read it and marks it
// here; the mark is the service's word, bound to the content, not a flag of this page.
export default function SkillPane({
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
	const [license, setLicense] = useState("");
	const [compatibility, setCompatibility] = useState("");
	const [allowedTools, setAllowedTools] = useState("");
	const [contentHash, setContentHash] = useState("");
	const [source, setSource] = useState("");
	const [body, setBody] = useState("");
	const [files, setFiles] = useState<readonly SkillFileView[]>([]);
	const [findings, setFindings] = useState<readonly LibraryFinding[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [notice, setNotice] = useState<string | null>(null);
	const [busy, setBusy] = useState(false);
	const [tab, setTab] = useState(name !== null ? "text" : "edit");
	// Read the skill again after a save or a mark, so the state, the findings and the hash the next mark sends
	// are those of the skill as it is now.
	const [version, setVersion] = useState(0);

	useEffect(() => {
		void version;
		let live = true;
		setFailure(null);
		if (name === null) {
			setEntry(null);
			setNewName("");
			setDescription("");
			setLicense("");
			setCompatibility("");
			setAllowedTools("");
			setContentHash("");
			setSource("");
			setBody("");
			setFiles([]);
			setFindings([]);
			return () => {
				live = false;
			};
		}
		client
			.getSkill(name)
			.then((result) => {
				if (!live) return;
				setEntry(result.entry);
				setDescription(result.entry.description);
				setLicense(result.license ?? "");
				setCompatibility(result.compatibility ?? "");
				setAllowedTools(result.allowed_tools ?? "");
				setContentHash(result.content_hash);
				setSource(result.source ?? "");
				setBody(result.body);
				setFiles(result.files ?? []);
				setFindings(result.findings ?? []);
			})
			.catch((error: unknown) => {
				if (live) setFailure(failureOf(error));
			});
		return () => {
			live = false;
		};
	}, [client, name, version]);

	async function save(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		const target = name ?? newName.trim();
		setBusy(true);
		setFailure(null);
		setNotice(null);
		try {
			const written = await client.writeSkill(target, {
				description,
				license: license.trim() === "" ? undefined : license.trim(),
				compatibility: compatibility === "" ? undefined : compatibility,
				allowed_tools: allowedTools === "" ? undefined : allowedTools,
				body,
				files: null,
			});
			setFindings(written.findings ?? []);
			setNotice(t("ui.library.saved"));
			setVersion((v) => v + 1);
			onChanged(written.entry.name);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	async function review(): Promise<void> {
		if (name === null) return;
		setBusy(true);
		setFailure(null);
		setNotice(null);
		try {
			await client.reviewSkill(name, contentHash);
			setNotice(t("ui.library.skill.reviewed_done"));
			setVersion((v) => v + 1);
			onChanged(name);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<div className="flex flex-col gap-3" data-testid="skill-pane">
			{entry !== null && (
				<div className="flex flex-col gap-2">
					<div className="flex flex-wrap items-center gap-2">
						<h2 className="text-lg font-semibold">{entry.name}</h2>
						<EntryBadges entry={entry} />
					</div>
					{entry.warning !== undefined && entry.warning !== "" && (
						<p role="status" className="text-sm text-destructive">
							{entry.warning}
						</p>
					)}
					<dl className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1 text-sm">
						<dt className="text-muted-foreground">
							{t("ui.library.skill.origin")}
						</dt>
						<dd>{t(`ui.library.origin.${entry.origin ?? "user"}`)}</dd>
						{source !== "" && (
							<>
								<dt className="text-muted-foreground">
									{t("ui.library.skill.source")}
								</dt>
								<dd>
									<UntrustedText>{source}</UntrustedText>
								</dd>
							</>
						)}
						<dt className="text-muted-foreground">
							{t("ui.library.field.license")}
						</dt>
						<dd>
							{license === "" ? (
								t("ui.library.skill.no_licence")
							) : (
								<UntrustedText>{license}</UntrustedText>
							)}
						</dd>
					</dl>
					{!entry.reviewed && (
						<div className="flex flex-col gap-1">
							<p className="text-sm">{t("ui.library.skill.review_hint")}</p>
							<div>
								<Button
									type="button"
									disabled={busy || !privileged || (entry.lint_errors ?? 0) > 0}
									aria-describedby={
										privileged ? undefined : "review-person-only"
									}
									onClick={() => void review()}
								>
									{t("ui.library.skill.mark_reviewed")}
								</Button>
								<PersonOnlyNote id="review-person-only" />
							</div>
						</div>
					)}
				</div>
			)}
			<Tabs value={tab} onValueChange={setTab}>
				<TabsList>
					{entry !== null && (
						<TabsTrigger value="text">{t("ui.library.skill.text")}</TabsTrigger>
					)}
					<TabsTrigger value="edit">{t("ui.library.tab.edit")}</TabsTrigger>
					{entry !== null && (
						<TabsTrigger value="lint">{t("ui.library.skill.lint")}</TabsTrigger>
					)}
				</TabsList>
				<TabsContent value="text">
					<div className="flex flex-col gap-3">
						<FileText
							label={t("ui.library.skill.full_text")}
							text={`${description}\n\n${body}`}
						/>
						{files.length > 0 && (
							<section aria-label={t("ui.library.skill.files")}>
								<h3 className="mb-1 text-sm font-medium">
									{t("ui.library.skill.files")}
								</h3>
								<ul className="flex flex-col gap-2 text-sm">
									{files.map((file) => (
										<li key={file.path} className="flex flex-col gap-1">
											<span>
												<UntrustedText>{file.path}</UntrustedText>{" "}
												<span className="text-muted-foreground">
													{t("ui.library.skill.bytes", { count: file.bytes })}
												</span>
												{file.script === true && (
													<span className="ml-2 text-destructive">
														{t("ui.library.skill.script")}
													</span>
												)}
											</span>
											{file.text !== undefined && (
												<FileText label={file.path} text={file.text} />
											)}
										</li>
									))}
								</ul>
							</section>
						)}
					</div>
				</TabsContent>
				<TabsContent value="edit">
					<form className="flex flex-col gap-3" onSubmit={save}>
						{name === null && (
							<div className="flex flex-col gap-1">
								<Label htmlFor="skill-name">{t("ui.library.field.name")}</Label>
								<Input
									id="skill-name"
									value={newName}
									maxLength={64}
									required
									onChange={(e) => setNewName(e.target.value)}
								/>
								<p className="text-xs text-muted-foreground">
									{t("ui.library.field.name_hint")}
								</p>
							</div>
						)}
						<div className="flex flex-col gap-1">
							<Label htmlFor="skill-description">
								{t("ui.library.field.description")}
							</Label>
							<Input
								id="skill-description"
								value={description}
								required
								onChange={(e) => setDescription(e.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="skill-license">
								{t("ui.library.field.license")}
							</Label>
							<Input
								id="skill-license"
								value={license}
								onChange={(e) => setLicense(e.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="skill-body">
								{t("ui.library.skill.instructions")}
							</Label>
							<Textarea
								id="skill-body"
								rows={12}
								value={body}
								required
								onChange={(e) => setBody(e.target.value)}
							/>
						</div>
						<div>
							<Button type="submit" disabled={busy}>
								{t("ui.library.action.save")}
							</Button>
						</div>
					</form>
				</TabsContent>
				<TabsContent value="lint">
					<Findings findings={findings} />
				</TabsContent>
			</Tabs>
			{notice !== null && (
				<p role="status" className="text-sm">
					{notice}
				</p>
			)}
			<FailureNote failure={failure} />
		</div>
	);
}
