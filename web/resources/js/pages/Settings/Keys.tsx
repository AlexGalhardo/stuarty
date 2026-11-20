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
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { ApiError } from "../../editor/api/errors";
import { HttpTransport } from "../../editor/api/http";
import type { ApiKey } from "../../editor/api/types";

// posts:read is the scope of the read API of the blog (SDD-020 section 47): a key that holds only it reads the published
// posts and nothing else, for the build of a static site.
const SCOPES = [
	"read",
	"edit",
	"media",
	"render",
	"admin",
	"posts:read",
] as const;
const NAME_MAX = 120;

// A session that ended goes to the login, which comes back here (SDD-012
// section 4.4); any other failure is shown on the page.
export function reportOrLogin(
	error: unknown,
	show: (failure: Failure) => void,
	goTo: (url: string) => void = (url) => window.location.assign(url),
): void {
	if (error instanceof ApiError && error.code === "E_UNAUTHORIZED") {
		goTo(`/login?next=${encodeURIComponent("/settings/keys")}`);
		return;
	}
	show(failureOf(error));
}

export function KeyRow({
	apiKey,
	onRevoke,
}: {
	apiKey: ApiKey;
	onRevoke(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const revoked = apiKey.revoked_at !== null;
	return (
		<li
			className="flex items-center justify-between gap-4 p-4"
			data-key-id={apiKey.key_id}
		>
			<div className="min-w-0">
				<p className="truncate font-medium">
					<UntrustedText>{apiKey.name}</UntrustedText>
				</p>
				<p className="text-xs text-muted-foreground">
					{apiKey.scopes.join(", ")}
					{apiKey.project_id !== null && ` · ${apiKey.project_id}`}
					{" · "}
					{t("ui.keys.created", {
						date: formatDateTime(apiKey.created_at, locale),
					})}
					{revoked && ` · ${t("ui.keys.revoked")}`}
				</p>
			</div>
			{!revoked && (
				<Button type="button" variant="outline" onClick={onRevoke}>
					{t("ui.keys.revoke")}
					<span className="sr-only">
						{" "}
						<UntrustedText>{apiKey.name}</UntrustedText>
					</span>
				</Button>
			)}
		</li>
	);
}

// The API keys page of network mode (SDD-012 section 2.1, SDD-007): create a key
// with a name, scopes and an optional project, and revoke one. The secret of a
// new key is shown once, in the state of this page only: it is never written to
// storage or to the URL, and it is gone when the person dismisses it or leaves.
export default function SettingsKeys(): ReactNode {
	const t = useT();
	const [keys, setKeys] = useState<ApiKey[] | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [secret, setSecret] = useState<string | null>(null);
	const [name, setName] = useState("");
	const [scopes, setScopes] = useState<string[]>(["read"]);
	const [projectId, setProjectId] = useState("");
	const [busy, setBusy] = useState(false);

	const load = useCallback(async (): Promise<void> => {
		try {
			setKeys(
				(await createClient(new HttpTransport("/api/v1")).listKeys()).keys,
			);
		} catch (error) {
			reportOrLogin(error, setFailure);
		}
	}, []);

	useEffect(() => {
		void load();
	}, [load]);

	async function create(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			const made = await createClient(new HttpTransport("/api/v1")).createKey({
				name: name.trim(),
				scopes,
				...(projectId.trim() === "" ? {} : { project_id: projectId.trim() }),
			});
			setSecret(made.secret);
			setName("");
			await load();
		} catch (error) {
			reportOrLogin(error, setFailure);
		} finally {
			setBusy(false);
		}
	}

	async function revoke(keyId: string): Promise<void> {
		setFailure(null);
		try {
			await createClient(new HttpTransport("/api/v1")).revokeKey(keyId);
			await load();
		} catch (error) {
			reportOrLogin(error, setFailure);
		}
	}

	return (
		<>
			<Head title={t("ui.keys.title")} />
			<div className="mx-auto flex max-w-3xl flex-col gap-8">
				<h1 className="text-2xl font-semibold">{t("ui.keys.title")}</h1>
				{secret !== null && (
					<div
						role="status"
						className="rounded-xl border bg-muted p-4 text-sm"
						data-key-secret=""
					>
						<p className="font-medium">{t("ui.keys.secret_once")}</p>
						<p className="mt-2 font-mono break-all">{secret}</p>
						<Button
							type="button"
							variant="outline"
							className="mt-3"
							onClick={() => setSecret(null)}
						>
							{t("ui.keys.secret_dismiss")}
						</Button>
					</div>
				)}
				<section aria-labelledby="key-new">
					<h2 id="key-new" className="text-lg font-semibold">
						{t("ui.keys.new")}
					</h2>
					<form
						onSubmit={(event) => void create(event)}
						className="mt-4 flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground"
					>
						<div className="flex flex-col gap-2">
							<Label htmlFor="key-name">{t("ui.keys.name")}</Label>
							<Input
								id="key-name"
								required
								maxLength={NAME_MAX}
								autoComplete="off"
								value={name}
								onChange={(event) => setName(event.target.value)}
							/>
						</div>
						<fieldset className="flex flex-wrap gap-4">
							<legend className="mb-2 text-sm font-medium">
								{t("ui.keys.scopes")}
							</legend>
							{SCOPES.map((scope) => (
								<label key={scope} className="flex items-center gap-2 text-sm">
									<input
										type="checkbox"
										className="accent-primary"
										checked={scopes.includes(scope)}
										onChange={(event) =>
											setScopes((was) =>
												event.target.checked
													? [...was, scope]
													: was.filter((s) => s !== scope),
											)
										}
									/>
									{scope}
								</label>
							))}
						</fieldset>
						<div className="flex flex-col gap-2">
							<Label htmlFor="key-project">{t("ui.keys.project")}</Label>
							<Input
								id="key-project"
								autoComplete="off"
								placeholder="prj_..."
								value={projectId}
								onChange={(event) => setProjectId(event.target.value)}
							/>
						</div>
						<div>
							<Button
								type="submit"
								disabled={busy || name.trim() === "" || scopes.length === 0}
							>
								{t("ui.keys.create")}
							</Button>
						</div>
					</form>
				</section>
				<section aria-labelledby="key-list">
					<h2 id="key-list" className="text-lg font-semibold">
						{t("ui.keys.list")}
					</h2>
					<FailureNote failure={failure} />
					{keys !== null &&
						(keys.length === 0 ? (
							<p className="mt-4 text-sm text-muted-foreground">
								{t("ui.keys.empty")}
							</p>
						) : (
							<ul className="mt-4 divide-y rounded-xl border bg-card text-card-foreground">
								{keys.map((apiKey) => (
									<KeyRow
										key={apiKey.key_id}
										apiKey={apiKey}
										onRevoke={() => void revoke(apiKey.key_id)}
									/>
								))}
							</ul>
						))}
				</section>
			</div>
		</>
	);
}
