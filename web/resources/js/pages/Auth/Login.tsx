import { Head } from "@inertiajs/react";
import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { safeNext } from "@/lib/safeNext";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";

// The login of network mode (SDD-012 section 2.1, SDD-007 section 4.1): email
// and password go to `POST /auth/login`, which sets the session cookie; the page
// then goes to `?next=` (set by the editor on E_UNAUTHORIZED) or the projects.
export default function Login(): ReactNode {
	const t = useT();
	const [email, setEmail] = useState("");
	const [password, setPassword] = useState("");
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);

	async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			await createClient(new HttpTransport("")).login(email.trim(), password);
			const next = new URLSearchParams(window.location.search).get("next");
			// The editor is another document, so this is a navigation, not an
			// Inertia visit (SDD-012 section 2.2).
			window.location.assign(safeNext(next));
		} catch (error) {
			setFailure(failureOf(error));
			setBusy(false);
		}
	}

	return (
		<>
			<Head title={t("ui.login.title")} />
			<div className="mx-auto flex max-w-sm flex-col gap-6">
				<h1 className="text-2xl font-semibold">{t("ui.login.title")}</h1>
				<form
					onSubmit={(event) => void submit(event)}
					className="flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground"
				>
					<div className="flex flex-col gap-2">
						<Label htmlFor="login-email">{t("ui.login.email")}</Label>
						<Input
							id="login-email"
							type="email"
							required
							autoComplete="username"
							value={email}
							onChange={(event) => setEmail(event.target.value)}
						/>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="login-password">{t("ui.login.password")}</Label>
						<Input
							id="login-password"
							type="password"
							required
							autoComplete="current-password"
							value={password}
							onChange={(event) => setPassword(event.target.value)}
						/>
					</div>
					<FailureNote failure={failure} />
					<div>
						<Button type="submit" disabled={busy}>
							{t("ui.login.submit")}
						</Button>
					</div>
				</form>
			</div>
		</>
	);
}
