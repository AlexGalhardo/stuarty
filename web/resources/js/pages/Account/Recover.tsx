import { Head } from "@inertiajs/react";
import { type FormEvent, type ReactNode, useState } from "react";

import { AuthCard } from "@/components/AuthCard";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import PasswordField from "@/components/PasswordField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { AccountFieldProblem } from "../../editor/api/types";
import { fieldProblems, reasonsOf } from "../../lib/accountProblems";

// A forgotten password (SDD-012 section 2.1.1, SDD-007 section 3.9 item 7): the recovery key opens the data and
// a new password replaces the old one. Without the password and without the key nothing can be recovered, and
// the page says so.
export default function Recover(): ReactNode {
	const t = useT();
	const [key, setKey] = useState("");
	const [password, setPassword] = useState("");
	const [confirmation, setConfirmation] = useState("");
	const [problems, setProblems] = useState<AccountFieldProblem[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);

	async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		setProblems([]);
		try {
			await createClient(new HttpTransport("")).accountRecover(
				key,
				password,
				confirmation,
			);
			window.location.assign("/");
		} catch (error) {
			const fields = fieldProblems(error);
			if (fields.length > 0) setProblems(fields);
			else setFailure(failureOf(error));
			setBusy(false);
		}
	}

	return (
		<>
			<Head title={t("ui.account.recover.title")} />
			<AuthCard
				title={t("ui.account.recover.title")}
				description={t("ui.account.recover.intro")}
				footer={t("ui.account.footer")}
			>
				<form
					onSubmit={(event) => void submit(event)}
					noValidate
					className="flex flex-col gap-4"
				>
					<div className="flex flex-col gap-2">
						<Label htmlFor="recover-key">{t("ui.account.recover.key")}</Label>
						<Input
							id="recover-key"
							autoComplete="off"
							autoCapitalize="characters"
							spellCheck={false}
							aria-invalid={failure === null ? undefined : true}
							aria-describedby={failure === null ? undefined : "recover-error"}
							value={key}
							onChange={(event) => setKey(event.target.value)}
						/>
					</div>
					<PasswordField
						id="recover-password"
						label={t("ui.account.recover.new_password")}
						value={password}
						onChange={setPassword}
						autoComplete="new-password"
						reasons={reasonsOf(problems, "password")}
					/>
					<PasswordField
						id="recover-confirmation"
						label={t("ui.account.confirmation")}
						value={confirmation}
						onChange={setConfirmation}
						autoComplete="new-password"
						reasons={reasonsOf(problems, "confirmation")}
					/>
					<FailureNote id="recover-error" failure={failure} />
					<Button type="submit" disabled={busy}>
						{t("ui.account.recover.submit")}
					</Button>
				</form>
			</AuthCard>
		</>
	);
}
