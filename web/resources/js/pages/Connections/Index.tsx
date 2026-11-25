import { Head, Link } from "@inertiajs/react";
import { Plug, Plus, ShieldCheck } from "lucide-react";
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
import { PageHeader, PageHeaderAction } from "@/components/PageHeader";
import { SectionTitle } from "@/components/SectionTitle";
import { StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	Connection,
	ConnectionTestResult,
	CostSummary,
	ProviderSignIn,
	ProviderView,
} from "../../editor/api/types";
import BlogSection from "./BlogSection";
import ProviderCards from "./ProviderCards";
import SocialSection from "./SocialSection";

const ACCEPTS = [
	"text",
	"image",
	"video",
	"audio",
	"metadata",
	"document",
] as const;
const LABEL_MAX = 60;

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

export function ConnectionRow({
	connection,
	onTest,
	onRemove,
	result,
	signedIn = false,
}: {
	connection: Connection;
	onTest(): void;
	onRemove(): void;
	result: ConnectionTestResult | null;
	// The server lists it as made by a sign-in, which only a person removes (SDD-014 section 13, T-75).
	signedIn?: boolean;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const [asking, setAsking] = useState(false);
	const { privileged } = useSession();
	const removeRefused = signedIn && !privileged;
	const reasonId = `remove-person-only-${connection.id}`;
	// The label is text an agent may have written: it is isolated inside the question, as it is in the row.
	const [titleBefore, titleAfter] = t("ui.connections.confirm_remove_title", {
		name: "\u0000",
	}).split("\u0000");
	return (
		<li className="flex flex-col gap-3 p-4" data-connection-id={connection.id}>
			<div className="flex items-start justify-between gap-4">
				<div className="min-w-0">
					<p className="truncate text-base font-medium text-text">
						<UntrustedText>{connection.label}</UntrustedText>
					</p>
					<p className="text-sm text-text-muted">
						{t("ui.connections.destinations", {
							hosts: connection.hosts.join(", "),
						})}
					</p>
					<p className="text-sm text-text-muted">
						{t("ui.connections.accepts", {
							list: connection.accepts
								.map((a) => t(`ui.connections.accept.${a}`))
								.join(", "),
						})}
					</p>
					<p className="text-sm text-text-subtle">
						{connection.last_used_at === null
							? t("ui.connections.never_used")
							: t("ui.connections.last_used", {
									date: formatDateTime(connection.last_used_at, locale),
								})}
					</p>
					<StatusBadge
						status={connection.secret_set ? "success" : "waiting"}
						className="mt-1"
					>
						{connection.secret_set
							? t("ui.connections.secret_set")
							: t("ui.connections.secret_missing")}
					</StatusBadge>
				</div>
				<div className="flex shrink-0 gap-2">
					<Button type="button" variant="outline" onClick={onTest}>
						{t("ui.connections.test")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{connection.label}</UntrustedText>
						</span>
					</Button>
					<Button
						type="button"
						variant="outline"
						disabled={removeRefused}
						aria-describedby={removeRefused ? reasonId : undefined}
						onClick={() => setAsking(true)}
					>
						{t("ui.connections.remove")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{connection.label}</UntrustedText>
						</span>
					</Button>
				</div>
			</div>
			{removeRefused && <PersonOnlyNote id={reasonId} />}
			{/* Removing deletes the credential from the vault and cannot be undone: it asks first. */}
			<AlertDialog open={asking && !removeRefused} onOpenChange={setAsking}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							{titleBefore}
							<UntrustedText className="[overflow-wrap:anywhere]">
								{connection.label}
							</UntrustedText>
							{titleAfter}
						</AlertDialogTitle>
						<AlertDialogDescription>
							{t("ui.connections.confirm_remove_body")}
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							{t("ui.connections.remove_cancel")}
						</AlertDialogCancel>
						<AlertDialogAction variant="destructive" onClick={onRemove}>
							{t("ui.connections.remove")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
			{result !== null && (
				<div
					role="status"
					className="rounded-lg border border-border bg-surface-sunken p-3 text-sm"
					data-test-result=""
				>
					<p className="font-medium">
						{result.ok
							? t("ui.connections.test_ok", { status: String(result.status) })
							: t("ui.connections.test_failed", {
									status: String(result.status),
								})}
					</p>
					<pre className="mt-1 whitespace-pre-wrap break-all">
						<UntrustedText>{result.message}</UntrustedText>
					</pre>
				</div>
			)}
		</li>
	);
}

// The Connections page (RF-220, SDD-014): every destination the product may reach, what it may
// receive and when it was last used, with the actions to add, test and remove one. The page shows
// what the service answers and decides nothing; the credential goes to the server in the form
// and is never shown again.
export default function Connections(): ReactNode {
	const t = useT();
	const [items, setItems] = useState<Connection[] | null>(null);
	const [providers, setProviders] = useState<ProviderView[]>([]);
	const [signIns, setSignIns] = useState<ProviderSignIn[]>([]);
	const [cost, setCost] = useState<CostSummary | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [results, setResults] = useState<Record<string, ConnectionTestResult>>(
		{},
	);
	const [label, setLabel] = useState("");
	const [endpoint, setEndpoint] = useState("");
	const [hosts, setHosts] = useState("");
	const [secret, setSecret] = useState("");
	const [accepts, setAccepts] = useState<string[]>(["text"]);
	const [busy, setBusy] = useState(false);
	// the form of a custom destination waits behind the "Add a connection" action
	const [adding, setAdding] = useState(false);

	const load = useCallback(async (): Promise<void> => {
		try {
			setItems((await api().listConnections()).connections);
			setProviders((await api().listProviders()).providers);
			// A build with no sign-in answers an empty list; a failure to read it leaves the cards without states.
			setSignIns(
				await api()
					.listProviderSignIns()
					.then((r) => r.sign_ins)
					.catch(() => []),
			);
			// The spend of a card is a number the service computed; a failure to read it leaves the cards without it.
			setCost(
				await api()
					.getCostSummary()
					.catch(() => null),
			);
		} catch (error) {
			setFailure(failureOf(error));
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
			const extra = hosts
				.split(",")
				.map((h) => h.trim())
				.filter((h) => h !== "");
			await api().createConnection({
				kind: "test",
				label: label.trim(),
				endpoint: endpoint.trim(),
				...(extra.length === 0 ? {} : { hosts: extra }),
				accepts,
				secret,
			});
			setLabel("");
			setEndpoint("");
			setHosts("");
			setSecret("");
			setAdding(false);
			await load();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	async function test(id: string): Promise<void> {
		setFailure(null);
		try {
			const result = await api().testConnection(id);
			setResults((was) => ({ ...was, [id]: result }));
			await load();
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	async function remove(id: string): Promise<void> {
		setFailure(null);
		try {
			await api().removeConnection(id);
			setResults(({ [id]: _gone, ...rest }) => rest);
			await load();
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	return (
		<>
			<Head title={t("ui.connections.title")} />
			<div className="mx-auto flex max-w-5xl flex-col gap-8">
				<PageHeader
					title={t("ui.connections.title")}
					description={t("ui.connections.intro")}
					actions={
						<>
							<Button asChild variant="outline">
								<Link href="/approvals">
									<ShieldCheck aria-hidden="true" />
									{t("ui.connections.approvals_link")}
								</Link>
							</Button>
							<PageHeaderAction
								type="button"
								data-testid="connection-new"
								onClick={() => setAdding(true)}
							>
								<Plus aria-hidden="true" />
								{t("ui.connections.add_action")}
							</PageHeaderAction>
						</>
					}
				/>
				<Notice
					tone="success"
					title={t("ui.connections.vault_title")}
					data-testid="connections-vault-note"
				>
					{t("ui.connections.vault_body")}
				</Notice>
				<FailureNote failure={failure} />
				{providers.length > 0 && (
					<ProviderCards
						providers={providers}
						connections={items ?? []}
						signIns={signIns}
						cost={cost}
						onChanged={() => void load()}
					/>
				)}
				<SocialSection />
				<BlogSection onChanged={() => void load()} />
				<section
					aria-label={t("ui.connections.list")}
					className="grid grid-cols-1 gap-4"
				>
					<SectionTitle
						title={t("ui.connections.list")}
						description={t("ui.connections.list_hint")}
					/>
					{items !== null &&
						(items.length === 0 ? (
							<EmptyState
								icon={Plug}
								level={3}
								title={t("ui.connections.empty_title")}
								description={t("ui.connections.empty")}
							>
								<Button type="button" onClick={() => setAdding(true)}>
									<Plus aria-hidden="true" />
									{t("ui.connections.add_action")}
								</Button>
							</EmptyState>
						) : (
							<ul className="divide-y divide-border rounded-lg border border-border bg-surface text-text">
								{items.map((connection) => (
									<ConnectionRow
										key={connection.id}
										connection={connection}
										result={results[connection.id] ?? null}
										signedIn={signIns.some(
											(s) => s.connection_id === connection.id,
										)}
										onTest={() => void test(connection.id)}
										onRemove={() => void remove(connection.id)}
									/>
								))}
							</ul>
						))}
				</section>
				{adding && (
					<section
						id="connections-new"
						aria-label={t("ui.connections.new")}
						className="grid scroll-mt-4 grid-cols-1 gap-4"
					>
						<SectionTitle title={t("ui.connections.new")} />
						<form
							onSubmit={(event) => void create(event)}
							className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-5 text-text"
						>
							<div className="flex flex-col gap-2">
								<Label htmlFor="connection-label">
									{t("ui.connections.label")}
								</Label>
								<Input
									id="connection-label"
									required
									maxLength={LABEL_MAX}
									autoComplete="off"
									value={label}
									onChange={(event) => setLabel(event.target.value)}
								/>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="connection-endpoint">
									{t("ui.connections.endpoint")}
								</Label>
								<Input
									id="connection-endpoint"
									required
									type="url"
									autoComplete="off"
									value={endpoint}
									onChange={(event) => setEndpoint(event.target.value)}
								/>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="connection-hosts">
									{t("ui.connections.hosts")}
								</Label>
								<Input
									id="connection-hosts"
									autoComplete="off"
									value={hosts}
									onChange={(event) => setHosts(event.target.value)}
								/>
							</div>
							<fieldset className="flex flex-wrap gap-4">
								<legend className="mb-2 text-sm font-medium">
									{t("ui.connections.accepts_legend")}
								</legend>
								{ACCEPTS.map((kind) => (
									<label key={kind} className="flex items-center gap-2 text-sm">
										<input
											type="checkbox"
											className="accent-primary"
											checked={accepts.includes(kind)}
											onChange={(event) =>
												setAccepts((was) =>
													event.target.checked
														? [...was, kind]
														: was.filter((a) => a !== kind),
												)
											}
										/>
										{t(`ui.connections.accept.${kind}`)}
									</label>
								))}
							</fieldset>
							<div className="flex flex-col gap-2">
								<Label htmlFor="connection-secret">
									{t("ui.connections.secret")}
								</Label>
								<Input
									id="connection-secret"
									required
									type="password"
									autoComplete="off"
									value={secret}
									onChange={(event) => setSecret(event.target.value)}
								/>
								<p className="text-sm text-text-subtle">
									{t("ui.connections.secret_hint")}
								</p>
							</div>
							<div>
								<Button
									type="submit"
									disabled={
										busy ||
										label.trim() === "" ||
										endpoint.trim() === "" ||
										secret === "" ||
										accepts.length === 0
									}
								>
									{t("ui.connections.add")}
								</Button>
							</div>
						</form>
					</section>
				)}
			</div>
		</>
	);
}
