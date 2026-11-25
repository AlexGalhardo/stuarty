import { Head } from "@inertiajs/react";
import { ArrowRight } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";

import { AuthCard, type AuthStep } from "@/components/AuthCard";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import FieldErrors from "@/components/FieldErrors";
import { Notice } from "@/components/Notice";
import PasswordField from "@/components/PasswordField";
import RecoveryKeyBox from "@/components/RecoveryKeyBox";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { AccountFieldProblem } from "../../editor/api/types";
import { fieldProblems, reasonsOf } from "../../lib/accountProblems";

// The three steps of the first run, with the one the person is on.
function createSteps(
	t: ReturnType<typeof useT>,
	current: "create" | "key",
): AuthStep[] {
	const done = {
		state: "done",
		stateLabel: t("ui.account.steps.done"),
	} as const;
	return [
		{
			label: t("ui.account.steps.create"),
			...(current === "create" ? { state: "current" as const } : done),
		},
		{
			label: t("ui.account.steps.key"),
			state: current === "key" ? "current" : "todo",
		},
		{ label: t("ui.account.steps.signin"), state: "todo" },
	];
}

// The first run (SDD-012 section 2.1.1, ADR-0052): the account, then the recovery key. The key is shown once,
// in the memory of this page only: it is not in the address, the history or the storage, and it is gone when
// the page is left. The way on is a box the person ticks to say it was saved.
export default function Create(): ReactNode {
	const t = useT();
	const [username, setUsername] = useState("");
	const [email, setEmail] = useState("");
	const [password, setPassword] = useState("");
	const [confirmation, setConfirmation] = useState("");
	const [problems, setProblems] = useState<AccountFieldProblem[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	const [recoveryKey, setRecoveryKey] = useState<string | null>(null);
	const [formerFolder, setFormerFolder] = useState<string | null>(null);

	async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		setProblems([]);
		try {
			const created = await createClient(new HttpTransport("")).createAccount({
				username,
				email,
				password,
				confirmation,
			});
			setPassword("");
			setConfirmation("");
			setFormerFolder(created.former_folder ?? null);
			setRecoveryKey(created.recovery_key);
		} catch (error) {
			const fields = fieldProblems(error);
			if (fields.length > 0) setProblems(fields);
			else setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	if (recoveryKey !== null) {
		return <RecoveryKey value={recoveryKey} formerFolder={formerFolder} />;
	}

	const usernameReasons = reasonsOf(problems, "username");
	const emailReasons = reasonsOf(problems, "email");

	return (
		<>
			<Head title={t("ui.account.create.title")} />
			<AuthCard
				title={t("ui.account.create.title")}
				description={t("ui.account.create.intro")}
				steps={createSteps(t, "create")}
				stepsLabel={t("ui.account.steps.label")}
				footer={t("ui.account.footer")}
			>
				<details
					data-testid="create-not-encrypted"
					className="rounded-lg border border-border bg-surface-sunken px-4 py-3 text-sm"
				>
					<summary className="cursor-pointer font-medium text-accent-text">
						{t("ui.account.create.not_encrypted_title")}
					</summary>
					<p className="mt-2 text-text-muted">
						{t("ui.account.create.encrypted")}
					</p>
					<p className="mt-2 text-text-muted">
						{t("ui.account.create.not_encrypted")}
					</p>
				</details>
				<form
					onSubmit={(event) => void submit(event)}
					noValidate
					className="flex flex-col gap-4"
				>
					<div className="flex flex-col gap-2">
						<Label htmlFor="account-username">{t("ui.account.username")}</Label>
						<Input
							id="account-username"
							autoComplete="username"
							autoCapitalize="none"
							spellCheck={false}
							aria-invalid={usernameReasons.length > 0 ? true : undefined}
							aria-describedby="account-username-hint account-username-errors"
							value={username}
							onChange={(event) => setUsername(event.target.value)}
						/>
						<p id="account-username-hint" className="text-sm text-text-subtle">
							{t("ui.account.username_rule")}
						</p>
						<FieldErrors
							id="account-username-errors"
							reasons={usernameReasons}
						/>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="account-email">{t("ui.account.email")}</Label>
						<Input
							id="account-email"
							type="email"
							autoComplete="email"
							aria-invalid={emailReasons.length > 0 ? true : undefined}
							aria-describedby="account-email-hint account-email-errors"
							value={email}
							onChange={(event) => setEmail(event.target.value)}
						/>
						<p id="account-email-hint" className="text-sm text-text-subtle">
							{t("ui.account.email_rule")}
						</p>
						<FieldErrors id="account-email-errors" reasons={emailReasons} />
					</div>
					<PasswordField
						id="account-password"
						label={t("ui.account.password")}
						value={password}
						onChange={setPassword}
						autoComplete="new-password"
						reasons={reasonsOf(problems, "password")}
					/>
					<p className="-mt-2 text-sm text-text-subtle">
						{t("ui.account.password_rule")}
					</p>
					<PasswordField
						id="account-confirmation"
						label={t("ui.account.confirmation")}
						value={confirmation}
						onChange={setConfirmation}
						autoComplete="new-password"
						reasons={reasonsOf(problems, "confirmation")}
					/>
					<FailureNote id="account-error" failure={failure} />
					<Button type="submit" size="lg" disabled={busy}>
						<ArrowRight aria-hidden="true" />
						{t("ui.account.create.submit")}
					</Button>
				</form>
			</AuthCard>
		</>
	);
}

// The recovery key, once. The words say plainly what losing it means.
export function RecoveryKey({
	value,
	formerFolder,
}: {
	value: string;
	formerFolder: string | null;
}): ReactNode {
	const t = useT();
	const [saved, setSaved] = useState(false);

	return (
		<>
			<Head title={t("ui.account.recovery.title")} />
			<AuthCard
				wide
				title={t("ui.account.recovery.title")}
				description={t("ui.account.recovery.intro")}
				steps={createSteps(t, "key")}
				stepsLabel={t("ui.account.steps.label")}
				footer={t("ui.account.footer")}
			>
				{formerFolder !== null && (
					<Notice
						tone="warning"
						title={t("ui.account.recovery.formerFolder_title")}
						data-testid="former-folder"
					>
						{t("ui.account.recovery.formerFolder", { path: formerFolder })}
					</Notice>
				)}
				<RecoveryKeyBox value={value} />
				<Notice tone="warning" title={t("ui.account.recovery.warning_title")}>
					{t("ui.account.recovery.warning")}
				</Notice>
				<div className="flex items-center gap-2">
					<input
						id="recovery-saved"
						type="checkbox"
						className="size-4 accent-primary"
						checked={saved}
						onChange={(event) => setSaved(event.target.checked)}
					/>
					<Label htmlFor="recovery-saved">
						{t("ui.account.recovery.saved")}
					</Label>
				</div>
				<Button
					type="button"
					size="lg"
					disabled={!saved}
					onClick={() => window.location.assign("/account/signin")}
				>
					<ArrowRight aria-hidden="true" />
					{t("ui.account.recovery.continue")}
				</Button>
			</AuthCard>
		</>
	);
}
