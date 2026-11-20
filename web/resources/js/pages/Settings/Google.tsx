import { Head } from "@inertiajs/react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { ApiError } from "../../editor/api/errors";
import { HttpTransport } from "../../editor/api/http";
import type {
	Connection,
	GoogleSettings,
	GoogleStatus,
} from "../../editor/api/types";

// A session that ended goes to the login, which comes back here; any other failure is shown on the page.
function report(error: unknown, show: (failure: Failure) => void): void {
	if (error instanceof ApiError && error.code === "E_UNAUTHORIZED") {
		window.location.assign(
			`/login?next=${encodeURIComponent("/settings/google")}`,
		);
		return;
	}
	show(failureOf(error));
}

// The optional Google sign-in (SDD-007 section 4.7, SDD-014 section 12): the person links or unlinks their
// own Google account (the password is asked again), and an administrator turns the feature on, chooses the
// connection of the Google project and may restrict it to a list of addresses. The page computes nothing.
export default function SettingsGoogle(): ReactNode {
	const t = useT();
	const api = createClient(new HttpTransport(""));
	const [status, setStatus] = useState<GoogleStatus | null>(null);
	const [settings, setSettings] = useState<GoogleSettings | null>(null);
	const [connections, setConnections] = useState<Connection[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [password, setPassword] = useState("");
	const [enabled, setEnabled] = useState(false);
	const [connectionId, setConnectionId] = useState("");
	const [allowed, setAllowed] = useState("");
	const [saved, setSaved] = useState(false);
	const [busy, setBusy] = useState(false);

	const load = useCallback(async (): Promise<void> => {
		try {
			setStatus(await createClient(new HttpTransport("")).googleStatus());
		} catch (error) {
			report(error, setFailure);
			return;
		}
		// Only an administrator session reads the settings; for anyone else the section stays hidden.
		try {
			const read = await createClient(new HttpTransport("")).googleSettings();
			setSettings(read);
			setEnabled(read.enabled);
			setConnectionId(read.connection_id);
			setAllowed(read.allowed_emails.join("\n"));
			setConnections(
				(
					await createClient(new HttpTransport("/api/v1")).listConnections()
				).connections.filter((c) => c.kind === "youtube"),
			);
		} catch (error) {
			if (!(error instanceof ApiError && error.code === "E_FORBIDDEN")) {
				report(error, setFailure);
			}
		}
	}, []);

	useEffect(() => {
		void load();
	}, [load]);

	async function link(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			const started = await api.googleLink(password);
			setPassword("");
			window.location.assign(started.authorize_url);
		} catch (error) {
			report(error, setFailure);
			setBusy(false);
		}
	}

	async function unlink(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			await api.googleUnlink(password);
			setPassword("");
			await load();
		} catch (error) {
			report(error, setFailure);
		} finally {
			setBusy(false);
		}
	}

	async function save(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		setSaved(false);
		try {
			const list = allowed
				.split(/\s*[\n,]\s*/)
				.map((s) => s.trim())
				.filter((s) => s !== "");
			setSettings(
				await api.setGoogleSettings({
					enabled,
					...(connectionId === "" ? {} : { connection_id: connectionId }),
					allowed_emails: list,
				}),
			);
			setSaved(true);
			await load();
		} catch (error) {
			report(error, setFailure);
		} finally {
			setBusy(false);
		}
	}

	const passwordField = (
		<div className="flex flex-col gap-2">
			<Label htmlFor="google-password">{t("ui.google.password")}</Label>
			<Input
				id="google-password"
				type="password"
				required
				autoComplete="current-password"
				value={password}
				onChange={(event) => setPassword(event.target.value)}
			/>
		</div>
	);

	return (
		<>
			<Head title={t("ui.google.title")} />
			<div className="mx-auto flex max-w-3xl flex-col gap-8">
				<h1 className="text-2xl font-semibold">{t("ui.google.title")}</h1>
				<FailureNote failure={failure} />
				{status !== null && !status.enabled && settings === null && (
					<p className="text-sm" data-google-off="">
						{t("ui.google.off")}
					</p>
				)}
				{status !== null && (status.enabled || settings !== null) && (
					<section
						aria-labelledby="google-account"
						className="flex flex-col gap-3 rounded-xl border bg-card p-5"
					>
						<h2 id="google-account" className="text-lg font-medium">
							{t("ui.google.account")}
						</h2>
						{status.linked !== null ? (
							<form
								onSubmit={(event) => void unlink(event)}
								className="flex flex-col gap-3"
							>
								<p className="text-sm" data-google-linked="">
									{t("ui.google.linked")}{" "}
									<UntrustedText>{status.linked.email}</UntrustedText>
								</p>
								<p className="text-xs text-muted-foreground">
									{t("ui.google.unlink_note")}
								</p>
								{passwordField}
								<div>
									<Button type="submit" variant="outline" disabled={busy}>
										{t("ui.google.unlink")}
									</Button>
								</div>
							</form>
						) : (
							<form
								onSubmit={(event) => void link(event)}
								className="flex flex-col gap-3"
							>
								<p className="text-sm">{t("ui.google.not_linked")}</p>
								{passwordField}
								<div>
									<Button type="submit" disabled={busy || !status.enabled}>
										{t("ui.google.link")}
									</Button>
								</div>
							</form>
						)}
					</section>
				)}
				{settings !== null && (
					<section
						aria-labelledby="google-owner"
						className="flex flex-col gap-3 rounded-xl border bg-card p-5"
					>
						<h2 id="google-owner" className="text-lg font-medium">
							{t("ui.google.owner")}
						</h2>
						<p className="text-sm">{t("ui.google.owner_intro")}</p>
						<p className="text-sm">
							{t("ui.google.redirect")}{" "}
							<code data-google-redirect="">{settings.redirect_url}</code>
						</p>
						<form
							onSubmit={(event) => void save(event)}
							className="flex flex-col gap-4"
						>
							<div className="flex items-center gap-2">
								<input
									id="google-enabled"
									type="checkbox"
									checked={enabled}
									onChange={(event) => setEnabled(event.target.checked)}
								/>
								<Label htmlFor="google-enabled">{t("ui.google.enabled")}</Label>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="google-connection">
									{t("ui.google.connection")}
								</Label>
								<select
									id="google-connection"
									className="h-9 rounded-md border bg-background px-3 text-sm"
									value={connectionId}
									onChange={(event) => setConnectionId(event.target.value)}
								>
									<option value="">{t("ui.google.connection_none")}</option>
									{connections.map((c) => (
										<option key={c.id} value={c.id}>
											{c.label}
										</option>
									))}
								</select>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="google-allowed">{t("ui.google.allowed")}</Label>
								<textarea
									id="google-allowed"
									rows={4}
									className="rounded-md border bg-background p-3 text-sm"
									value={allowed}
									onChange={(event) => setAllowed(event.target.value)}
								/>
								<p className="text-xs text-muted-foreground">
									{t("ui.google.allowed_hint")}
								</p>
							</div>
							<div className="flex items-center gap-3">
								<Button type="submit" disabled={busy}>
									{t("ui.google.save")}
								</Button>
								{saved && (
									<span role="status" className="text-sm" data-google-saved="">
										{t("ui.google.saved")}
									</span>
								)}
							</div>
						</form>
					</section>
				)}
			</div>
		</>
	);
}
