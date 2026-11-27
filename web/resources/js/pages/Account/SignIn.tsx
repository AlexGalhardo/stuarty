import { Head, usePage } from "@inertiajs/react";
import { LockOpen } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";

import { AuthCard } from "@/components/AuthCard";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import PasswordField from "@/components/PasswordField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type { SharedProps } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";

// The sign-in of the local account (SDD-012 section 2.1.1, SDD-007 section 3.9 item 3): the username or the
// e-mail and the password unlock the studio. The password goes to the server and nowhere else.
export default function SignIn(): ReactNode {
	const t = useT();
	// With an account the studio already knows, the card names it and only the password is asked;
	// the person can always switch to another account, even when the studio is locked (epic 103).
	const account = usePage<SharedProps>().props.account;
	const known = account?.username ? account.username : null;
	const [other, setOther] = useState(false);
	const asKnown = known !== null && !other;
	const [identity, setIdentity] = useState("");
	const [password, setPassword] = useState("");
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);

	async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			await createClient(new HttpTransport("")).accountLogin(
				asKnown ? known : identity.trim(),
				password,
			);
			// The dashboard is another document with a session of its own, so this is a navigation.
			window.location.assign("/");
		} catch (error) {
			setFailure(failureOf(error));
			setPassword("");
			setBusy(false);
		}
	}

	return (
		<>
			<Head title={t("ui.account.signin.title")} />
			<AuthCard
				title={
					asKnown
						? t("ui.account.signin.welcome")
						: t("ui.account.signin.title")
				}
				description={
					asKnown
						? t("ui.account.signin.welcome_intro")
						: t("ui.account.signin.intro_open")
				}
				footer={t("ui.account.footer")}
			>
				<form
					onSubmit={(event) => void submit(event)}
					className="flex flex-col gap-4"
				>
					{asKnown ? (
						<div
							data-testid="signin-account"
							className="flex items-center gap-3 rounded-lg bg-surface-sunken px-4 py-3"
						>
							<span
								aria-hidden="true"
								className="flex size-9 shrink-0 items-center justify-center rounded-full bg-accent-soft text-sm font-semibold text-accent-text"
							>
								{known.slice(0, 2).toUpperCase()}
							</span>
							<p className="min-w-0 truncate text-sm font-medium">{known}</p>
						</div>
					) : (
						<div className="flex flex-col gap-2">
							<Label htmlFor="signin-identity">
								{t("ui.account.identity")}
							</Label>
							<Input
								id="signin-identity"
								required
								autoComplete="username"
								autoCapitalize="none"
								spellCheck={false}
								aria-invalid={failure === null ? undefined : true}
								aria-describedby={failure === null ? undefined : "signin-error"}
								value={identity}
								onChange={(event) => setIdentity(event.target.value)}
							/>
						</div>
					)}
					<PasswordField
						id="signin-password"
						label={t("ui.account.password")}
						value={password}
						onChange={setPassword}
						autoComplete="current-password"
					/>
					<FailureNote id="signin-error" failure={failure} />
					<Button type="submit" size="lg" disabled={busy}>
						<LockOpen aria-hidden="true" />
						{t("ui.account.signin.submit")}
					</Button>
				</form>
				<p className="flex flex-col items-center gap-2 text-sm">
					{known !== null && !other ? (
						<button
							type="button"
							data-testid="signin-switch-account"
							className="underline underline-offset-4 max-md:py-3"
							onClick={() => setOther(true)}
						>
							{t("ui.account.signin.other_account")}
						</button>
					) : null}
					{known !== null && other ? (
						<button
							type="button"
							data-testid="signin-use-known"
							className="underline underline-offset-4 max-md:py-3"
							onClick={() => setOther(false)}
						>
							{t("ui.account.signin.use_known", { name: known })}
						</button>
					) : null}
					<a
						className="underline underline-offset-4 max-md:inline-block max-md:py-3"
						href="/account/recover"
					>
						{t("ui.account.signin.forgot")}
					</a>
				</p>
			</AuthCard>
		</>
	);
}
