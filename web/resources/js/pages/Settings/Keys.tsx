import { Head } from "@inertiajs/react";
import { Copy, KeyRound, Plus, ShieldCheck } from "lucide-react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useState,
} from "react";

import { EmptyState } from "@/components/EmptyState";
import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Notice } from "@/components/Notice";
import { PageHeader } from "@/components/PageHeader";
import { SectionTitle } from "@/components/SectionTitle";
import { StatusBadge } from "@/components/StatusBadge";
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
			className="flex flex-wrap items-center gap-3 py-3"
			data-key-id={apiKey.key_id}
		>
			<span
				aria-hidden="true"
				className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-surface-hover text-icon"
			>
				<KeyRound className="size-4" />
			</span>
			<div className="min-w-0 flex-1">
				<p className="truncate text-base font-medium text-text">
					<UntrustedText>{apiKey.name}</UntrustedText>
				</p>
				<p className="text-sm text-text-muted">
					{apiKey.scopes.join(", ")}
					{apiKey.project_id !== null && ` · ${apiKey.project_id}`}
					{" · "}
					{t("ui.keys.created", {
						date: formatDateTime(apiKey.created_at, locale),
					})}
				</p>
			</div>
			<StatusBadge status={revoked ? "neutral" : "success"}>
				{revoked ? t("ui.keys.revoked") : t("ui.keys.status_active")}
			</StatusBadge>
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

// copyText puts the text on the clipboard and says whether it did. A page served over plain HTTP from another
// machine (network mode) has no clipboard at all: the secret stays on the page to be selected, and nothing throws.
export async function copyText(value: string): Promise<boolean> {
	try {
		await navigator.clipboard.writeText(value);
		return true;
	} catch {
		return false;
	}
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
	const [copied, setCopied] = useState(false);
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
			setCopied(false);
			setName("");
			await load();
		} catch (error) {
			reportOrLogin(error, setFailure);
		} finally {
			setBusy(false);
		}
	}

	async function copySecret(value: string): Promise<void> {
		setCopied(await copyText(value));
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
			<div className="mx-auto flex max-w-6xl flex-col gap-6">
				<PageHeader
					title={t("ui.keys.title")}
					description={t("ui.keys.intro")}
					actions={
						<Button asChild>
							<a href="#key-new">
								<Plus aria-hidden="true" />
								{t("ui.keys.new_action")}
							</a>
						</Button>
					}
				/>
				{secret !== null && (
					<Notice
						tone="warning"
						role="status"
						title={t("ui.keys.secret_title")}
						data-testid="key-secret-note"
						actions={
							<>
								<Button
									type="button"
									variant="outline"
									onClick={() => void copySecret(secret)}
								>
									<Copy aria-hidden="true" />
									{copied
										? t("ui.keys.secret_copied")
										: t("ui.keys.secret_copy")}
								</Button>
								<Button
									type="button"
									variant="outline"
									onClick={() => setSecret(null)}
								>
									{t("ui.keys.secret_dismiss")}
								</Button>
							</>
						}
					>
						<p>{t("ui.keys.secret_once")}</p>
						<p
							className="mt-2 rounded-lg bg-surface-sunken p-2 font-mono break-all text-text"
							data-key-secret=""
						>
							{secret}
						</p>
					</Notice>
				)}
				<div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
					<section
						aria-label={t("ui.keys.list")}
						className="grid grid-cols-1 gap-4 rounded-lg border border-border bg-surface p-5"
					>
						<SectionTitle level={3} title={t("ui.keys.list")} />
						<FailureNote failure={failure} />
						{keys !== null &&
							(keys.length === 0 ? (
								<EmptyState
									icon={KeyRound}
									level={3}
									title={t("ui.keys.empty_title")}
									description={t("ui.keys.empty")}
								/>
							) : (
								<ul className="divide-y divide-border">
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
					<section
						id="key-new"
						aria-label={t("ui.keys.new")}
						className="scroll-mt-4 rounded-lg border border-border bg-surface p-5"
					>
						<SectionTitle level={3} title={t("ui.keys.new")} />
						<form
							onSubmit={(event) => void create(event)}
							className="mt-4 flex flex-col gap-4"
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
							<fieldset className="grid grid-cols-2 gap-x-4 gap-y-2">
								<legend className="mb-2 text-sm font-medium text-text">
									{t("ui.keys.scopes")}
								</legend>
								{SCOPES.map((scope) => (
									<label
										key={scope}
										className="flex items-center gap-2 text-sm max-md:min-h-11"
									>
										<input
											type="checkbox"
											className="size-4 accent-primary"
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
							<p className="flex items-start gap-2 text-sm text-text-muted">
								<ShieldCheck
									aria-hidden="true"
									className="mt-0.5 size-4 shrink-0"
								/>
								{t("ui.keys.no_approval")}
							</p>
							<Button
								type="submit"
								disabled={busy || name.trim() === "" || scopes.length === 0}
							>
								<KeyRound aria-hidden="true" />
								{t("ui.keys.create")}
							</Button>
						</form>
					</section>
				</div>
			</div>
		</>
	);
}
