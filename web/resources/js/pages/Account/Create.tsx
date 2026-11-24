import { Head } from "@inertiajs/react";
import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import FieldErrors from "@/components/FieldErrors";
import PasswordField from "@/components/PasswordField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { AccountFieldProblem } from "../../editor/api/types";
import { fieldProblems, reasonsOf } from "../../lib/accountProblems";

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
			<div className="mx-auto flex max-w-sm flex-col gap-6">
				<h1 className="text-2xl font-semibold">
					{t("ui.account.create.title")}
				</h1>
				<p className="text-sm text-muted-foreground">
					{t("ui.account.create.intro")} {t("ui.account.create.not_encrypted")}
				</p>
				<form
					onSubmit={(event) => void submit(event)}
					noValidate
					className="flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground"
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
						<p
							id="account-username-hint"
							className="text-sm text-muted-foreground"
						>
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
						<p
							id="account-email-hint"
							className="text-sm text-muted-foreground"
						>
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
					<p className="-mt-2 text-sm text-muted-foreground">
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
					<div>
						<Button type="submit" disabled={busy}>
							{t("ui.account.create.submit")}
						</Button>
					</div>
				</form>
			</div>
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
	const [copied, setCopied] = useState(false);

	async function copy(): Promise<void> {
		try {
			await navigator.clipboard.writeText(value);
			setCopied(true);
		} catch {
			setCopied(false);
		}
	}

	return (
		<>
			<Head title={t("ui.account.recovery.title")} />
			<div className="mx-auto flex max-w-lg flex-col gap-6">
				<h1 className="text-2xl font-semibold">
					{t("ui.account.recovery.title")}
				</h1>
				<p className="text-sm">{t("ui.account.recovery.intro")}</p>
				<p
					role="note"
					className="rounded-lg border border-destructive p-3 text-sm"
				>
					{t("ui.account.recovery.warning")}
				</p>
				{formerFolder !== null && (
					<p
						role="note"
						data-testid="former-folder"
						className="rounded-lg border border-destructive p-3 text-sm"
					>
						{t("ui.account.recovery.formerFolder", { path: formerFolder })}
					</p>
				)}
				<p
					data-testid="recovery-key"
					className="rounded-lg border bg-muted p-4 text-center font-mono text-lg tracking-wide break-all select-all"
				>
					{value}
				</p>
				<div className="flex items-center gap-3">
					<Button type="button" variant="outline" onClick={() => void copy()}>
						{t("ui.account.recovery.copy")}
					</Button>
					<span role="status" className="text-sm text-muted-foreground">
						{copied ? t("ui.account.recovery.copied") : ""}
					</span>
				</div>
				<div className="flex items-center gap-2">
					<input
						id="recovery-saved"
						type="checkbox"
						className="size-4"
						checked={saved}
						onChange={(event) => setSaved(event.target.checked)}
					/>
					<Label htmlFor="recovery-saved">
						{t("ui.account.recovery.saved")}
					</Label>
				</div>
				<div>
					<Button
						type="button"
						disabled={!saved}
						onClick={() => window.location.assign("/account/signin")}
					>
						{t("ui.account.recovery.continue")}
					</Button>
				</div>
			</div>
		</>
	);
}
