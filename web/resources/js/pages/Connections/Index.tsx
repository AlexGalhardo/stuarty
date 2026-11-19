import { Head, Link } from "@inertiajs/react";
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
import { HttpTransport } from "../../editor/api/http";
import type { Connection, ConnectionTestResult } from "../../editor/api/types";

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
}: {
	connection: Connection;
	onTest(): void;
	onRemove(): void;
	result: ConnectionTestResult | null;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	return (
		<li className="flex flex-col gap-3 p-4" data-connection-id={connection.id}>
			<div className="flex items-start justify-between gap-4">
				<div className="min-w-0">
					<p className="truncate font-medium">
						<UntrustedText>{connection.label}</UntrustedText>
					</p>
					<p className="text-xs text-muted-foreground">
						{t("ui.connections.destinations", {
							hosts: connection.hosts.join(", "),
						})}
					</p>
					<p className="text-xs text-muted-foreground">
						{t("ui.connections.accepts", {
							list: connection.accepts
								.map((a) => t(`ui.connections.accept.${a}`))
								.join(", "),
						})}
					</p>
					<p className="text-xs text-muted-foreground">
						{connection.last_used_at === null
							? t("ui.connections.never_used")
							: t("ui.connections.last_used", {
									date: formatDateTime(connection.last_used_at, locale),
								})}
						{" · "}
						{connection.secret_set
							? t("ui.connections.secret_set")
							: t("ui.connections.secret_missing")}
					</p>
				</div>
				<div className="flex shrink-0 gap-2">
					<Button type="button" variant="outline" onClick={onTest}>
						{t("ui.connections.test")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{connection.label}</UntrustedText>
						</span>
					</Button>
					<Button type="button" variant="outline" onClick={onRemove}>
						{t("ui.connections.remove")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{connection.label}</UntrustedText>
						</span>
					</Button>
				</div>
			</div>
			{result !== null && (
				<div
					role="status"
					className="rounded-lg border bg-muted p-3 text-xs"
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

	const load = useCallback(async (): Promise<void> => {
		try {
			setItems((await api().listConnections()).connections);
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
			<div className="mx-auto flex max-w-3xl flex-col gap-8">
				<div>
					<h1 className="text-2xl font-semibold">
						{t("ui.connections.title")}
					</h1>
					<p className="mt-1 text-sm text-muted-foreground">
						{t("ui.connections.intro")}
					</p>
					<p className="mt-2 text-sm">
						<Link href="/approvals" className="underline underline-offset-4">
							{t("ui.connections.approvals_link")}
						</Link>
					</p>
				</div>
				<section aria-labelledby="connections-list">
					<h2 id="connections-list" className="text-lg font-semibold">
						{t("ui.connections.list")}
					</h2>
					<FailureNote failure={failure} />
					{items !== null &&
						(items.length === 0 ? (
							<p className="mt-4 text-sm text-muted-foreground">
								{t("ui.connections.empty")}
							</p>
						) : (
							<ul className="mt-4 divide-y rounded-xl border bg-card text-card-foreground">
								{items.map((connection) => (
									<ConnectionRow
										key={connection.id}
										connection={connection}
										result={results[connection.id] ?? null}
										onTest={() => void test(connection.id)}
										onRemove={() => void remove(connection.id)}
									/>
								))}
							</ul>
						))}
				</section>
				<section aria-labelledby="connections-new">
					<h2 id="connections-new" className="text-lg font-semibold">
						{t("ui.connections.new")}
					</h2>
					<form
						onSubmit={(event) => void create(event)}
						className="mt-4 flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground"
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
							<p className="text-xs text-muted-foreground">
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
			</div>
		</>
	);
}
