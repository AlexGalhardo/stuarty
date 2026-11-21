import { Head } from "@inertiajs/react";
import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import PasswordField from "@/components/PasswordField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";

// The sign-in of the local account (SDD-012 section 2.1.1, SDD-007 section 3.9 item 3): the username or the
// e-mail and the password unlock the studio. The password goes to the server and nowhere else.
export default function SignIn(): ReactNode {
	const t = useT();
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
				identity.trim(),
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
			<div className="mx-auto flex max-w-sm flex-col gap-6">
				<h1 className="text-2xl font-semibold">
					{t("ui.account.signin.title")}
				</h1>
				<form
					onSubmit={(event) => void submit(event)}
					className="flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground"
				>
					<div className="flex flex-col gap-2">
						<Label htmlFor="signin-identity">{t("ui.account.identity")}</Label>
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
					<PasswordField
						id="signin-password"
						label={t("ui.account.password")}
						value={password}
						onChange={setPassword}
						autoComplete="current-password"
					/>
					<FailureNote id="signin-error" failure={failure} />
					<div>
						<Button type="submit" disabled={busy}>
							{t("ui.account.signin.submit")}
						</Button>
					</div>
				</form>
				<p className="text-sm">
					<a className="underline" href="/account/recover">
						{t("ui.account.signin.forgot")}
					</a>
				</p>
			</div>
		</>
	);
}
