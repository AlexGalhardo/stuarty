import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import type {
	AgentProfile,
	AgentProfileListResult,
	AgentSuggestion,
} from "../../editor/api/types";
import { fullCommand, parseArgs } from "./fullCommand";

// What the panel shows when no agent is connected, and the way to connect one (SDD-017 section 3,
// RF-382). Stuarty hosts the agent the user already has: it never installs one and never signs in
// to one. A profile is a command that will run on this machine, so the form shows it in full
// before it is saved, and only an administrator in the app may write one.
export default function ProfileSetup({
	profiles,
	busy,
	failure,
	onSave,
	onRemove,
}: {
	profiles: AgentProfileListResult;
	busy: boolean;
	failure: Failure | null;
	onSave(name: string, command: string, args: string[]): Promise<boolean>;
	onRemove(name: string): void;
}): ReactNode {
	const t = useT();
	const { privileged } = useSession();
	const [name, setName] = useState("");
	const [command, setCommand] = useState("");
	const [args, setArgs] = useState("");
	const parsed = parseArgs(args);

	const use = (s: AgentSuggestion): void => {
		setName(s.name);
		setCommand(s.found_path ?? s.executables[0] ?? "");
		setArgs(s.args.join("\n"));
	};

	const submit = async (event: FormEvent): Promise<void> => {
		event.preventDefault();
		if (await onSave(name.trim(), command.trim(), parsed)) {
			setName("");
			setCommand("");
			setArgs("");
		}
	};

	return (
		<div className="flex flex-col gap-4" data-testid="chat-setup">
			<FailureNote failure={failure} />
			{profiles.profiles.length === 0 && (
				<div>
					<h3 className="font-medium">{t("ui.chat.no_profile_title")}</h3>
					<p className="mt-1 text-sm text-muted-foreground">
						{t("ui.chat.no_profile_text")}
					</p>
				</div>
			)}
			{!profiles.available && (
				<p role="status" className="text-sm text-muted-foreground">
					{t("ui.chat.not_available")}
				</p>
			)}
			{profiles.profiles.length > 0 && (
				<ul className="flex flex-col gap-2" data-testid="chat-profiles">
					{profiles.profiles.map((p) => (
						<ProfileRow
							key={p.name}
							profile={p}
							canWrite={profiles.can_write}
							busy={busy}
							onRemove={onRemove}
						/>
					))}
				</ul>
			)}
			<section aria-labelledby="chat-known-title">
				<h3 id="chat-known-title" className="text-sm font-medium">
					{t("ui.chat.known_title")}
				</h3>
				<ul className="mt-2 flex flex-col gap-2">
					{profiles.suggestions.map((s) => (
						<li
							key={s.name}
							className="rounded-lg border px-3 py-2 text-sm"
							data-testid="chat-suggestion"
						>
							<p className="font-medium">
								{s.title}{" "}
								<span className="text-xs font-normal text-muted-foreground">
									{s.found ? t("ui.chat.found") : t("ui.chat.not_found")}
								</span>
							</p>
							<p className="text-xs text-muted-foreground">{s.install}</p>
							{profiles.can_write && (
								<Button
									size="xs"
									variant="outline"
									className="mt-2"
									onClick={() => use(s)}
								>
									{t("ui.chat.use_suggestion")}
								</Button>
							)}
						</li>
					))}
				</ul>
			</section>
			{profiles.can_write ? (
				<form
					onSubmit={(event) => void submit(event)}
					className="flex flex-col gap-3"
					data-testid="chat-profile-form"
				>
					<h3 className="text-sm font-medium">{t("ui.chat.connect_title")}</h3>
					<div className="flex flex-col gap-1">
						<Label htmlFor="chat-profile-name">
							{t("ui.chat.profile_name")}
						</Label>
						<Input
							id="chat-profile-name"
							value={name}
							maxLength={32}
							autoComplete="off"
							onChange={(event) => setName(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor="chat-profile-command">
							{t("ui.chat.profile_command")}
						</Label>
						<Input
							id="chat-profile-command"
							value={command}
							maxLength={1024}
							autoComplete="off"
							spellCheck={false}
							onChange={(event) => setCommand(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-1">
						<Label htmlFor="chat-profile-args">
							{t("ui.chat.profile_args")}
						</Label>
						<Textarea
							id="chat-profile-args"
							value={args}
							spellCheck={false}
							onChange={(event) => setArgs(event.target.value)}
						/>
					</div>
					<div>
						<p className="text-xs text-muted-foreground">
							{t("ui.chat.full_command")}
						</p>
						<p
							className="mt-1 rounded bg-muted p-2 font-mono text-xs break-all"
							data-testid="chat-full-command"
						>
							<UntrustedText>
								{command.trim() === ""
									? ""
									: fullCommand(command.trim(), parsed)}
							</UntrustedText>
						</p>
						<p className="mt-1 text-xs text-muted-foreground">
							{t("ui.chat.profile_warning")}
						</p>
					</div>
					<Button
						type="submit"
						disabled={busy || name.trim() === "" || command.trim() === ""}
					>
						{t("ui.chat.save_profile")}
					</Button>
				</form>
			) : privileged ? (
				<p
					className="text-sm text-muted-foreground"
					data-testid="chat-ask-admin"
				>
					{t("ui.chat.ask_admin")}
				</p>
			) : (
				<PersonOnlyNote id="chat-profile-person-only" />
			)}
		</div>
	);
}

function ProfileRow({
	profile,
	canWrite,
	busy,
	onRemove,
}: {
	profile: AgentProfile;
	canWrite: boolean;
	busy: boolean;
	onRemove(name: string): void;
}): ReactNode {
	const t = useT();
	return (
		<li
			className="flex items-start justify-between gap-2 rounded-lg border px-3 py-2"
			data-testid="chat-profile"
		>
			<div className="min-w-0">
				<p className="text-sm font-medium">
					<UntrustedText>{profile.name}</UntrustedText>
				</p>
				{profile.command !== "" && (
					<p className="font-mono text-xs break-all text-muted-foreground">
						<UntrustedText>
							{fullCommand(profile.command, profile.args)}
						</UntrustedText>
					</p>
				)}
			</div>
			{canWrite && (
				<Button
					size="xs"
					variant="ghost"
					disabled={busy}
					onClick={() => onRemove(profile.name)}
				>
					{t("ui.chat.remove_profile")}
				</Button>
			)}
		</li>
	);
}
