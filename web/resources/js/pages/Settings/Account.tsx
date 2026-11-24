import { usePage } from "@inertiajs/react";
import { type FormEvent, type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import PasswordField from "@/components/PasswordField";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type { SharedProps } from "@/types";
import { createClient } from "../../editor/api/client";
import { ApiError } from "../../editor/api/errors";
import { HttpTransport } from "../../editor/api/http";
import type { AccountFieldProblem } from "../../editor/api/types";
import { fieldProblems, reasonsOf } from "../../lib/accountProblems";

// The idle lock a person may choose, in minutes; 0 is off. The server checks the range (SDD-007 section 3.9).
const IDLE_CHOICES = [0, 5, 15, 30, 60, 240, 1440];

// The account in Settings (SDD-012 section 2.1.1): who is signed in, the password and the idle lock. These
// are for a person who typed the password; a browser opened by a launch link is told to sign in.
export default function AccountSettings(): ReactNode {
	const t = useT();
	const account = usePage<SharedProps>().props.account;
	const [idle, setIdle] = useState<number | null>(null);
	const [person, setPerson] = useState<boolean | null>(null);
	const [current, setCurrent] = useState("");
	const [password, setPassword] = useState("");
	const [confirmation, setConfirmation] = useState("");
	const [problems, setProblems] = useState<AccountFieldProblem[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [saved, setSaved] = useState<"idle" | "password" | null>(null);

	useEffect(() => {
		if (account?.state !== "unlocked") return;
		let live = true;
		createClient(new HttpTransport(""))
			.accountSettings()
			.then((result) => {
				if (!live) return;
				setPerson(true);
				setIdle(result.idle_lock_minutes ?? 0);
			})
			.catch((error) => {
				if (!live) return;
				if (error instanceof ApiError && error.code === "E_PRIVILEGED_SESSION")
					setPerson(false);
				else setFailure(failureOf(error));
			});
		return () => {
			live = false;
		};
	}, [account?.state]);

	if (account?.state !== "unlocked") return null;

	async function changeIdle(minutes: number): Promise<void> {
		setFailure(null);
		setSaved(null);
		try {
			const result = await createClient(
				new HttpTransport(""),
			).setAccountIdleLock(minutes);
			setIdle(result.idle_lock_minutes ?? minutes);
			setSaved("idle");
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	async function changePassword(
		event: FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault();
		setFailure(null);
		setProblems([]);
		setSaved(null);
		try {
			await createClient(new HttpTransport("")).accountChangePassword(
				current,
				password,
				confirmation,
			);
			setCurrent("");
			setPassword("");
			setConfirmation("");
			setSaved("password");
		} catch (error) {
			const fields = fieldProblems(error);
			if (fields.length > 0) setProblems(fields);
			else setFailure(failureOf(error));
		}
	}

	return (
		<section
			aria-labelledby="settings-account"
			className="flex flex-col gap-4"
			data-testid="settings-account"
		>
			<h2 id="settings-account" className="text-lg font-semibold">
				{t("ui.account.settings.title")}
			</h2>
			<p className="text-sm">
				{t("ui.account.settings.signed_in")}{" "}
				<strong data-testid="settings-username">{account.username}</strong>
			</p>
			<p
				role="note"
				className="text-sm text-muted-foreground"
				data-testid="settings-not-encrypted"
			>
				{t("ui.account.create.not_encrypted")}
			</p>
			{person === false && (
				<p role="note" className="text-sm text-muted-foreground">
					{t("ui.account.settings.need_password")}{" "}
					<a className="underline" href="/account/signin">
						{t("ui.account.signin.title")}
					</a>
				</p>
			)}
			{person === true && (
				<>
					<div className="flex items-center justify-between gap-4">
						<Label htmlFor="settings-idle">
							{t("ui.account.settings.idle")}
						</Label>
						<select
							id="settings-idle"
							value={idle ?? 0}
							className="h-8 rounded-lg border border-input bg-transparent px-2.5 text-sm dark:bg-input/30"
							onChange={(event) => void changeIdle(Number(event.target.value))}
						>
							{IDLE_CHOICES.map((minutes) => (
								<option key={minutes} value={minutes}>
									{minutes === 0
										? t("ui.account.settings.idle_off")
										: t("ui.account.settings.idle_minutes", { n: minutes })}
								</option>
							))}
						</select>
					</div>
					<p className="text-sm text-muted-foreground">
						{t("ui.account.settings.idle_hint")}
					</p>
					<form
						onSubmit={(event) => void changePassword(event)}
						noValidate
						className="flex flex-col gap-4"
					>
						<h3 className="text-base font-medium">
							{t("ui.account.settings.change")}
						</h3>
						<PasswordField
							id="settings-current"
							label={t("ui.account.settings.current")}
							value={current}
							onChange={setCurrent}
							autoComplete="current-password"
						/>
						<PasswordField
							id="settings-new"
							label={t("ui.account.settings.new")}
							value={password}
							onChange={setPassword}
							autoComplete="new-password"
							reasons={reasonsOf(problems, "password")}
						/>
						<PasswordField
							id="settings-confirm"
							label={t("ui.account.confirmation")}
							value={confirmation}
							onChange={setConfirmation}
							autoComplete="new-password"
							reasons={reasonsOf(problems, "confirmation")}
						/>
						<div>
							<Button type="submit">
								{t("ui.account.settings.change_submit")}
							</Button>
						</div>
					</form>
				</>
			)}
			<FailureNote id="settings-account-error" failure={failure} />
			<p role="status" className="text-sm text-muted-foreground">
				{saved === "idle" ? t("ui.account.settings.idle_saved") : ""}
				{saved === "password" ? t("ui.account.settings.password_saved") : ""}
			</p>
		</section>
	);
}
