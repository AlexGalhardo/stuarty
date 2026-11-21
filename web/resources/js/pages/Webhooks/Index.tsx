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
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	Webhook,
	WebhookDelivery,
	WebhookEvent,
} from "../../editor/api/types";

const NAME_MAX = 60;

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// The key of the localized name of an event: the dots of the event name become underscores.
function eventKey(name: string): string {
	return name.replace(/\./g, "_");
}

// The secret as the creation or the rotation answered it. It is held here, in memory, until the person has copied it and
// closes the note: the server cannot show it again.
export interface RevealedSecret {
	id: string;
	name: string;
	secret: string;
}

export function SecretNote({
	revealed,
	onClose,
}: {
	revealed: RevealedSecret;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const [copied, setCopied] = useState(false);

	async function copy(): Promise<void> {
		try {
			await navigator.clipboard.writeText(revealed.secret);
			setCopied(true);
		} catch {
			setCopied(false);
		}
	}

	return (
		<div
			role="status"
			className="mt-4 flex flex-col gap-3 rounded-xl border border-primary bg-card p-4 text-card-foreground"
			data-webhook-secret-note=""
		>
			<p className="font-medium">
				{t("ui.webhooks.secret_title", { name: revealed.name })}
			</p>
			<p className="text-sm">{t("ui.webhooks.secret_warning")}</p>
			<code
				className="break-all rounded-lg bg-muted p-3 text-xs"
				data-webhook-secret=""
			>
				{revealed.secret}
			</code>
			<div className="flex gap-2">
				<Button type="button" variant="outline" onClick={() => void copy()}>
					{copied ? t("ui.webhooks.copied") : t("ui.webhooks.copy")}
				</Button>
				<Button type="button" onClick={onClose}>
					{t("ui.webhooks.secret_saved")}
				</Button>
			</div>
		</div>
	);
}

// An action that cannot be undone: removing deletes the secret and the history, and rotating makes the receiver refuse
// every call until it holds the new secret. Both ask first.
export interface PendingAction {
	kind: "remove" | "rotate";
	id: string;
	name: string;
}

export function ConfirmAction({
	action,
	onCancel,
	onConfirm,
}: {
	action: PendingAction | null;
	onCancel(): void;
	onConfirm(): void;
}): ReactNode {
	const t = useT();
	const kind = action?.kind ?? "remove";
	return (
		<AlertDialog
			open={action !== null}
			onOpenChange={(next) => {
				if (!next) onCancel();
			}}
		>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>
						{t(`ui.webhooks.confirm_${kind}_title`, {
							name: action?.name ?? "",
						})}
					</AlertDialogTitle>
					<AlertDialogDescription>
						{t(`ui.webhooks.confirm_${kind}_body`)}
					</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>{t("ui.webhooks.cancel")}</AlertDialogCancel>
					<AlertDialogAction variant="destructive" onClick={onConfirm}>
						{t(`ui.webhooks.${kind}`)}
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}

export function DeliveryRow({
	delivery,
	onRedeliver,
}: {
	delivery: WebhookDelivery;
	onRedeliver(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	return (
		<li
			className="flex flex-col gap-1 p-3 text-sm"
			data-delivery-id={delivery.id}
			data-delivery-state={delivery.state}
		>
			<div className="flex items-start justify-between gap-4">
				<div className="min-w-0">
					<p className="font-medium">
						<code>{delivery.event}</code>
						{" · "}
						{t(`ui.webhooks.state.${delivery.state}`)}
					</p>
					<p className="text-xs text-muted-foreground">
						{formatDateTime(delivery.at, locale)}
						{" · "}
						{t("ui.webhooks.attempts", {
							n: String(delivery.attempts),
							max: String(delivery.max_attempts),
						})}
						{" · "}
						{t("ui.webhooks.status", { status: String(delivery.last_status) })}
					</p>
					{delivery.attempt_log.length > 0 && (
						<ul className="mt-1 text-xs text-muted-foreground">
							{delivery.attempt_log.map((a) => (
								<li key={a.n}>
									{t("ui.webhooks.attempt_line", {
										n: String(a.n),
										status: String(a.status),
										ms: String(a.duration_ms),
										error: a.error ?? "",
									})}
								</li>
							))}
						</ul>
					)}
				</div>
				<Button type="button" variant="outline" onClick={onRedeliver}>
					{t("ui.webhooks.redeliver")}
					<span className="sr-only"> {delivery.id}</span>
				</Button>
			</div>
		</li>
	);
}

// The Webhooks page (SDD-022 section 9): the addresses that are told when something happens, their events, and the
// deliveries of each with the actions to send a test and to send one again. The page shows what the service answers and
// decides nothing; the signing secret comes once and is shown once.
export default function Webhooks(): ReactNode {
	const t = useT();
	const locale = useLocale();
	const [items, setItems] = useState<Webhook[] | null>(null);
	const [events, setEvents] = useState<WebhookEvent[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [revealed, setRevealed] = useState<RevealedSecret | null>(null);
	const [open, setOpen] = useState<string | null>(null);
	const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
	const [name, setName] = useState("");
	const [url, setUrl] = useState("");
	const [chosen, setChosen] = useState<string[]>([]);
	const [busy, setBusy] = useState(false);
	const [confirming, setConfirming] = useState<PendingAction | null>(null);

	const load = useCallback(async (): Promise<void> => {
		try {
			setItems((await api().listWebhooks()).webhooks);
			setEvents((await api().listWebhookEvents()).events);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}, []);

	const loadDeliveries = useCallback(async (id: string): Promise<void> => {
		try {
			setDeliveries((await api().listWebhookDeliveries(id)).deliveries);
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
			const made = await api().createWebhook({
				name: name.trim(),
				url: url.trim(),
				events: chosen,
			});
			setRevealed({ id: made.id, name: made.name, secret: made.secret });
			setName("");
			setUrl("");
			setChosen([]);
			await load();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	async function act(work: () => Promise<void>): Promise<void> {
		setFailure(null);
		try {
			await work();
			await load();
			if (open !== null) await loadDeliveries(open);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	function confirmed(): void {
		const action = confirming;
		setConfirming(null);
		if (action === null) return;
		void act(async () => {
			if (action.kind === "remove") {
				await api().removeWebhook(action.id);
				if (open === action.id) setOpen(null);
				return;
			}
			const made = await api().rotateWebhookSecret(action.id);
			setRevealed({ id: made.id, name: made.name, secret: made.secret });
		});
	}

	async function show(id: string): Promise<void> {
		if (open === id) {
			setOpen(null);
			return;
		}
		setOpen(id);
		setDeliveries([]);
		await loadDeliveries(id);
	}

	return (
		<>
			<Head title={t("ui.webhooks.title")} />
			<div className="mx-auto flex max-w-3xl flex-col gap-8">
				<div>
					<h1 className="text-2xl font-semibold">{t("ui.webhooks.title")}</h1>
					<p className="mt-1 text-sm text-muted-foreground">
						{t("ui.webhooks.intro")}
					</p>
				</div>
				<section aria-labelledby="webhooks-list">
					<h2 id="webhooks-list" className="text-lg font-semibold">
						{t("ui.webhooks.list")}
					</h2>
					<FailureNote failure={failure} />
					{revealed !== null && (
						<SecretNote revealed={revealed} onClose={() => setRevealed(null)} />
					)}
					{items !== null &&
						(items.length === 0 ? (
							<p className="mt-4 text-sm text-muted-foreground">
								{t("ui.webhooks.empty")}
							</p>
						) : (
							<ul className="mt-4 divide-y rounded-xl border bg-card text-card-foreground">
								{items.map((w) => (
									<li
										key={w.id}
										className="flex flex-col gap-3 p-4"
										data-webhook-id={w.id}
									>
										<div className="flex items-start justify-between gap-4">
											<div className="min-w-0">
												<p className="truncate font-medium">
													<UntrustedText>{w.name}</UntrustedText>
												</p>
												<p className="truncate text-xs text-muted-foreground">
													{w.url}
												</p>
												<p className="text-xs text-muted-foreground">
													{w.events
														.map((e) => t(`ui.webhooks.event.${eventKey(e)}`))
														.join(", ")}
												</p>
												<p className="text-xs text-muted-foreground">
													{w.enabled
														? t("ui.webhooks.on")
														: t("ui.webhooks.off")}
													{" · "}
													{w.last_delivery === null
														? t("ui.webhooks.no_delivery")
														: t("ui.webhooks.last_delivery", {
																state: t(
																	`ui.webhooks.state.${w.last_delivery.state}`,
																),
																date: formatDateTime(
																	w.last_delivery.at,
																	locale,
																),
															})}
												</p>
											</div>
											<div className="flex shrink-0 flex-wrap justify-end gap-2">
												<Button
													type="button"
													variant="outline"
													onClick={() => void show(w.id)}
													aria-expanded={open === w.id}
												>
													{t("ui.webhooks.deliveries")}
													<span className="sr-only">
														{" "}
														<UntrustedText>{w.name}</UntrustedText>
													</span>
												</Button>
												<Button
													type="button"
													variant="outline"
													onClick={() =>
														void act(async () => {
															await api().testWebhook(w.id);
														})
													}
												>
													{t("ui.webhooks.test")}
													<span className="sr-only">
														{" "}
														<UntrustedText>{w.name}</UntrustedText>
													</span>
												</Button>
												<Button
													type="button"
													variant="outline"
													onClick={() =>
														void act(async () => {
															await api().updateWebhook(w.id, {
																enabled: !w.enabled,
															});
														})
													}
												>
													{w.enabled
														? t("ui.webhooks.turn_off")
														: t("ui.webhooks.turn_on")}
													<span className="sr-only">
														{" "}
														<UntrustedText>{w.name}</UntrustedText>
													</span>
												</Button>
												<Button
													type="button"
													variant="outline"
													onClick={() =>
														setConfirming({
															kind: "rotate",
															id: w.id,
															name: w.name,
														})
													}
												>
													{t("ui.webhooks.rotate")}
													<span className="sr-only">
														{" "}
														<UntrustedText>{w.name}</UntrustedText>
													</span>
												</Button>
												<Button
													type="button"
													variant="outline"
													onClick={() =>
														setConfirming({
															kind: "remove",
															id: w.id,
															name: w.name,
														})
													}
												>
													{t("ui.webhooks.remove")}
													<span className="sr-only">
														{" "}
														<UntrustedText>{w.name}</UntrustedText>
													</span>
												</Button>
											</div>
										</div>
										{open === w.id &&
											(deliveries.length === 0 ? (
												<p className="text-sm text-muted-foreground">
													{t("ui.webhooks.no_deliveries")}
												</p>
											) : (
												<ul
													className="divide-y rounded-lg border"
													aria-label={t("ui.webhooks.deliveries")}
												>
													{deliveries.map((d) => (
														<DeliveryRow
															key={d.id}
															delivery={d}
															onRedeliver={() =>
																void act(async () => {
																	await api().redeliverWebhook(w.id, d.id);
																})
															}
														/>
													))}
												</ul>
											))}
									</li>
								))}
							</ul>
						))}
				</section>
				<section aria-labelledby="webhooks-new">
					<h2 id="webhooks-new" className="text-lg font-semibold">
						{t("ui.webhooks.new")}
					</h2>
					<form
						onSubmit={(event) => void create(event)}
						className="mt-4 flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground"
					>
						<div className="flex flex-col gap-2">
							<Label htmlFor="webhook-name">{t("ui.webhooks.name")}</Label>
							<Input
								id="webhook-name"
								required
								maxLength={NAME_MAX}
								autoComplete="off"
								value={name}
								onChange={(event) => setName(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="webhook-url">{t("ui.webhooks.url")}</Label>
							<Input
								id="webhook-url"
								required
								type="url"
								autoComplete="off"
								value={url}
								onChange={(event) => setUrl(event.target.value)}
							/>
							<p className="text-xs text-muted-foreground">
								{t("ui.webhooks.url_hint")}
							</p>
						</div>
						<fieldset className="flex flex-col gap-3">
							<legend className="mb-1 text-sm font-medium">
								{t("ui.webhooks.events_legend")}
							</legend>
							{events.map((e) => (
								<label key={e.name} className="flex items-start gap-2 text-sm">
									<input
										type="checkbox"
										className="mt-1 accent-primary"
										checked={chosen.includes(e.name)}
										onChange={(event) =>
											setChosen((was) =>
												event.target.checked
													? [...was, e.name]
													: was.filter((n) => n !== e.name),
											)
										}
									/>
									<span>
										<span className="font-medium">
											{t(`ui.webhooks.event.${eventKey(e.name)}`)}
										</span>{" "}
										<code className="text-xs">{e.name}</code>
										<span
											className="block text-xs text-muted-foreground"
											lang="en"
										>
											{e.description}
										</span>
									</span>
								</label>
							))}
						</fieldset>
						<div>
							<Button
								type="submit"
								disabled={
									busy ||
									name.trim() === "" ||
									url.trim() === "" ||
									chosen.length === 0
								}
							>
								{t("ui.webhooks.add")}
							</Button>
						</div>
					</form>
				</section>
			</div>
			<ConfirmAction
				action={confirming}
				onCancel={() => setConfirming(null)}
				onConfirm={confirmed}
			/>
		</>
	);
}
