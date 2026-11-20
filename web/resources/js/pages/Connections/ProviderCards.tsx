import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import {
	formatMoney,
	microsToInput,
	parseMicros,
} from "../../dashboard/approval/money";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	Connection,
	CostSummary,
	ModelView,
	ProviderModelsResult,
	ProviderSignIn,
	ProviderView,
} from "../../editor/api/types";

const LABEL_MAX = 60;

// The providers that have a sign-in of their own text on the button and in the hint; any other with the
// sign-in credential kind has the generic one.
const SIGN_IN_TEXTS = ["openai", "openrouter"];

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// Begin the sign-in of a provider and go to its consent screen. The service builds the address and
// checks the redirect; the page only sends the person there.
async function beginSignIn(
	provider: string,
	connectionId?: string,
): Promise<void> {
	const result = await api().beginProviderSignIn({
		provider,
		redirect_uri: `${window.location.origin}/auth/callback`,
		...(connectionId === undefined ? {} : { connection_id: connectionId }),
	});
	window.location.assign(result.authorize_url);
}

// One model of the catalogue of a provider, with the price that would be used and where it comes
// from. The name is text the provider wrote, so it is shown as text and never as markup; a model
// with no price says so, because nothing can be generated with it until a person sets one.
export function ModelRow({
	model,
	onPrice,
}: {
	model: ModelView;
	onPrice(
		model: ModelView,
		currency: string,
		input: number,
		output: number,
		unit?: "image" | "mchar" | "second",
	): Promise<void>;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const { privileged } = useSession();
	// A model an adapter hints as an image, a voice or a video model is priced per image, per million characters
	// or per second (SDD-016 section 4.5); the rest, per million tokens. The price shown is the one in that unit.
	const unit =
		model.capability === "image"
			? "image"
			: model.capability === "voice"
				? "mchar"
				: model.capability === "video"
					? "second"
					: undefined;
	const shown =
		model.price !== null && model.price.unit === unit ? model.price : null;
	const [unitInput, setUnitInput] = useState(
		shown?.unit_micros === undefined ? "" : microsToInput(shown.unit_micros),
	);
	const unitMicros = parseMicros(unitInput);
	const [open, setOpen] = useState(false);
	const [currency, setCurrency] = useState(model.price?.currency ?? "USD");
	const [input, setInput] = useState(
		model.price === null
			? ""
			: microsToInput(model.price.input_micros_per_mtok),
	);
	const [output, setOutput] = useState(
		model.price === null
			? ""
			: microsToInput(model.price.output_micros_per_mtok),
	);
	const inputMicros = parseMicros(input);
	const outputMicros = parseMicros(output);

	return (
		<li className="flex flex-col gap-2 py-3" data-model-id={model.id}>
			<div className="flex items-start justify-between gap-3">
				<div className="min-w-0">
					<p className="truncate font-mono text-sm">{model.id}</p>
					{model.name !== "" && model.name !== model.id && (
						<p className="truncate text-xs text-muted-foreground">
							<UntrustedText>{model.name}</UntrustedText>
						</p>
					)}
					{model.context_tokens > 0 && (
						<p className="text-xs text-muted-foreground">
							{t("ui.providers.limits", {
								context: model.context_tokens.toLocaleString(locale),
								output: model.output_tokens.toLocaleString(locale),
							})}
						</p>
					)}
					<p className="text-xs">
						{shown === null
							? t("ui.providers.price_none")
							: unit !== undefined && shown.unit_micros !== undefined
								? t(`ui.providers.price_unit.${unit}`, {
										amount: formatMoney(
											{ micros: shown.unit_micros, currency: shown.currency },
											locale,
										),
										source: t(`ui.providers.source.${model.price_source}`),
									})
								: t("ui.providers.price", {
										input: formatMoney(
											{
												micros: shown.input_micros_per_mtok,
												currency: shown.currency,
											},
											locale,
										),
										output: formatMoney(
											{
												micros: shown.output_micros_per_mtok,
												currency: shown.currency,
											},
											locale,
										),
										source: t(`ui.providers.source.${model.price_source}`),
									})}
					</p>
				</div>
				<Button
					type="button"
					variant="outline"
					disabled={!privileged}
					aria-describedby={
						privileged ? undefined : `price-person-only-${model.id}`
					}
					onClick={() => setOpen(!open)}
					aria-expanded={open}
				>
					{t("ui.providers.set_price")}
					<span className="sr-only"> {model.id}</span>
				</Button>
				<PersonOnlyNote id={`price-person-only-${model.id}`} />
			</div>
			{open && (
				<form
					className="flex flex-wrap items-end gap-3 rounded-lg border bg-muted p-3"
					onSubmit={(event) => {
						event.preventDefault();
						if (unit !== undefined) {
							if (unitMicros === null) return;
							void onPrice(
								model,
								currency.trim().toUpperCase(),
								unitMicros,
								0,
								unit,
							);
							return;
						}
						if (inputMicros === null || outputMicros === null) return;
						void onPrice(
							model,
							currency.trim().toUpperCase(),
							inputMicros,
							outputMicros,
						);
					}}
				>
					<div className="flex flex-col gap-1">
						<Label htmlFor={`price-currency-${model.id}`}>
							{t("ui.providers.price_currency")}
						</Label>
						<Input
							id={`price-currency-${model.id}`}
							className="w-24"
							maxLength={3}
							value={currency}
							onChange={(event) => setCurrency(event.target.value)}
						/>
					</div>
					{unit === undefined ? (
						<>
							<div className="flex flex-col gap-1">
								<Label htmlFor={`price-in-${model.id}`}>
									{t("ui.providers.price_input")}
								</Label>
								<Input
									id={`price-in-${model.id}`}
									className="w-36"
									inputMode="decimal"
									value={input}
									onChange={(event) => setInput(event.target.value)}
								/>
							</div>
							<div className="flex flex-col gap-1">
								<Label htmlFor={`price-out-${model.id}`}>
									{t("ui.providers.price_output")}
								</Label>
								<Input
									id={`price-out-${model.id}`}
									className="w-36"
									inputMode="decimal"
									value={output}
									onChange={(event) => setOutput(event.target.value)}
								/>
							</div>
						</>
					) : (
						<div className="flex flex-col gap-1">
							<Label htmlFor={`price-unit-${model.id}`}>
								{t(`ui.providers.price_per.${unit}`)}
							</Label>
							<Input
								id={`price-unit-${model.id}`}
								className="w-36"
								inputMode="decimal"
								value={unitInput}
								onChange={(event) => setUnitInput(event.target.value)}
							/>
						</div>
					)}
					<Button
						type="submit"
						disabled={
							(unit === undefined
								? inputMicros === null || outputMicros === null
								: unitMicros === null) || currency.trim().length !== 3
						}
					>
						{t("ui.providers.price_save")}
					</Button>
				</form>
			)}
		</li>
	);
}

// The connections of one provider on one card: test the key, read the models, and the spend of the
// day, which is a number the service computed.
function ProviderConnection({
	connection,
	signIn,
	cost,
	onChanged,
}: {
	connection: Connection;
	signIn: ProviderSignIn | undefined;
	cost: CostSummary | null;
	onChanged(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const [models, setModels] = useState<ProviderModelsResult | null>(null);
	const [status, setStatus] = useState<string | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

	async function signInAgain(): Promise<void> {
		setFailure(null);
		try {
			await beginSignIn(connection.kind, connection.id);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	async function readModels(refresh: boolean): Promise<void> {
		setFailure(null);
		try {
			setModels(await api().listProviderModels(connection.id, refresh));
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	async function check(): Promise<void> {
		setFailure(null);
		try {
			const result = await api().testConnection(connection.id);
			setStatus(
				result.ok
					? t("ui.connections.test_ok", { status: String(result.status) })
					: t("ui.connections.test_failed", { status: String(result.status) }),
			);
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	async function setPrice(
		model: ModelView,
		currency: string,
		input: number,
		output: number,
		unit?: "image" | "mchar" | "second",
	): Promise<void> {
		setFailure(null);
		try {
			await api().setProviderPrice({
				provider: models?.provider ?? connection.kind,
				model: model.id,
				currency,
				...(unit === undefined
					? { input_micros_per_mtok: input, output_micros_per_mtok: output }
					: { unit, unit_micros: input }),
			});
			await readModels(false);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	const today = (cost?.days ?? []).filter(
		(line) => line.connection_id === connection.id && line.day === cost?.day,
	);

	return (
		<div
			className="flex flex-col gap-3 p-4"
			data-provider-connection={connection.id}
		>
			<div className="flex flex-wrap items-start justify-between gap-3">
				<div className="min-w-0">
					<p className="truncate font-medium">
						<UntrustedText>{connection.label}</UntrustedText>
					</p>
					<p className="text-xs text-muted-foreground">
						{today.length === 0
							? t("ui.providers.spend_none")
							: t("ui.providers.spend", {
									amount: today
										.map((line) => formatMoney(line, locale))
										.join(", "),
								})}
					</p>
					{signIn !== undefined && (
						<p
							className="text-xs"
							data-signin-state={signIn.state}
							role={signIn.state === "needs_sign_in" ? "alert" : undefined}
						>
							{signIn.state === "ready" ? (
								<>
									{t("ui.providers.signin_ready")}
									{signIn.account !== undefined && signIn.account !== "" && (
										<>
											{" "}
											<UntrustedText>{signIn.account}</UntrustedText>
										</>
									)}
								</>
							) : (
								<>
									<strong>{t("ui.providers.signin_needs")}</strong>{" "}
									{t("ui.providers.signin_needs_help")}
								</>
							)}
						</p>
					)}
				</div>
				<div className="flex shrink-0 gap-2">
					{signIn !== undefined && (
						<Button
							type="button"
							variant={signIn.state === "ready" ? "outline" : "default"}
							onClick={() => void signInAgain()}
						>
							{t("ui.providers.signin_again")}
							<span className="sr-only">
								{" "}
								<UntrustedText>{connection.label}</UntrustedText>
							</span>
						</Button>
					)}
					<Button type="button" variant="outline" onClick={() => void check()}>
						{t("ui.providers.check")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{connection.label}</UntrustedText>
						</span>
					</Button>
					<Button
						type="button"
						variant="outline"
						onClick={() => void readModels(models !== null)}
					>
						{models === null
							? t("ui.providers.models")
							: t("ui.providers.models_refresh")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{connection.label}</UntrustedText>
						</span>
					</Button>
				</div>
			</div>
			{status !== null && (
				<p role="status" className="text-xs" data-provider-status="">
					{status}
				</p>
			)}
			<FailureNote failure={failure} />
			{models !== null && (
				<div>
					<p className="text-xs text-muted-foreground">
						{t("ui.providers.fetched", {
							date: formatDateTime(models.fetched_at, locale),
						})}
					</p>
					{models.models.length === 0 ? (
						<p className="mt-2 text-sm">{t("ui.providers.models_empty")}</p>
					) : (
						<ul className="mt-2 divide-y">
							{models.models.map((model) => (
								<ModelRow key={model.id} model={model} onPrice={setPrice} />
							))}
						</ul>
					)}
				</div>
			)}
		</div>
	);
}

// One card for one provider: where to find the key, the form that connects it, and the connections
// that exist. The key goes to the server in the form and is never shown again.
export function ProviderCard({
	provider,
	connections,
	signIns,
	cost,
	onChanged,
}: {
	provider: ProviderView;
	connections: Connection[];
	signIns: ProviderSignIn[];
	cost: CostSummary | null;
	onChanged(): void;
}): ReactNode {
	const t = useT();
	const canSignIn = provider.credential_kinds.includes("oauth");
	const signInText = SIGN_IN_TEXTS.includes(provider.id)
		? provider.id
		: "generic";
	const [signInFailure, setSignInFailure] = useState<Failure | null>(null);

	async function startSignIn(): Promise<void> {
		setSignInFailure(null);
		try {
			await beginSignIn(provider.id);
		} catch (error) {
			setSignInFailure(failureOf(error));
		}
	}
	const [label, setLabel] = useState(provider.name);
	const [endpoint, setEndpoint] = useState(provider.default_endpoint);
	const [secret, setSecret] = useState("");
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const mine = connections.filter((c) => provider.connections.includes(c.id));

	async function connect(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			await api().createConnection({
				kind: provider.id,
				label: label.trim(),
				endpoint: endpoint.trim(),
				accepts: ["text"],
				secret,
			});
			setSecret("");
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<li
			className="flex flex-col rounded-xl border bg-card text-card-foreground"
			data-provider={provider.id}
		>
			<div className="flex flex-wrap items-baseline justify-between gap-2 p-4">
				<h3 className="text-base font-semibold">{provider.name}</h3>
				<a
					href={provider.docs_url}
					target="_blank"
					rel="noreferrer noopener"
					className="text-xs underline underline-offset-4"
				>
					{t("ui.providers.docs")}
				</a>
			</div>
			{mine.length === 0 ? (
				<p className="px-4 text-sm text-muted-foreground">
					{t("ui.providers.none")}
				</p>
			) : (
				<div className="divide-y border-t">
					{mine.map((connection) => (
						<ProviderConnection
							key={connection.id}
							connection={connection}
							signIn={signIns.find((s) => s.connection_id === connection.id)}
							cost={cost}
							onChanged={onChanged}
						/>
					))}
				</div>
			)}
			{canSignIn && (
				<div className="flex flex-col gap-2 border-t p-4">
					<p className="text-sm font-medium">
						{t("ui.providers.signin_title")}
					</p>
					<p className="text-xs text-muted-foreground">
						{t(`ui.providers.signin_hint.${signInText}`)}
					</p>
					<FailureNote failure={signInFailure} />
					<div>
						<Button
							type="button"
							data-provider-signin={provider.id}
							onClick={() => void startSignIn()}
						>
							{t(`ui.providers.signin.${signInText}`, {
								provider: provider.name,
							})}
						</Button>
					</div>
				</div>
			)}
			<form
				onSubmit={(event) => void connect(event)}
				className="flex flex-col gap-3 border-t p-4"
			>
				<p className="text-sm font-medium">
					{t("ui.providers.connect_title", { provider: provider.name })}
				</p>
				<div className="flex flex-col gap-1">
					<Label htmlFor={`provider-label-${provider.id}`}>
						{t("ui.providers.connect_name")}
					</Label>
					<Input
						id={`provider-label-${provider.id}`}
						required
						maxLength={LABEL_MAX}
						autoComplete="off"
						value={label}
						onChange={(event) => setLabel(event.target.value)}
					/>
				</div>
				<div className="flex flex-col gap-1">
					<Label htmlFor={`provider-endpoint-${provider.id}`}>
						{t("ui.providers.connect_address")}
					</Label>
					<Input
						id={`provider-endpoint-${provider.id}`}
						required
						type="url"
						autoComplete="off"
						value={endpoint}
						onChange={(event) => setEndpoint(event.target.value)}
					/>
				</div>
				<div className="flex flex-col gap-1">
					<Label htmlFor={`provider-key-${provider.id}`}>
						{t("ui.providers.connect_key")}
					</Label>
					<Input
						id={`provider-key-${provider.id}`}
						required
						type="password"
						autoComplete="off"
						value={secret}
						onChange={(event) => setSecret(event.target.value)}
					/>
					<p className="text-xs text-muted-foreground">
						{t("ui.providers.connect_hint")}
					</p>
				</div>
				<FailureNote failure={failure} />
				<div>
					<Button
						type="submit"
						disabled={
							busy ||
							label.trim() === "" ||
							endpoint.trim() === "" ||
							secret === ""
						}
					>
						{t("ui.providers.connect_submit")}
						<span className="sr-only"> {provider.name}</span>
					</Button>
				</div>
			</form>
		</li>
	);
}

// The AI providers section of the Connections page (RF-360, SDD-016): one card per provider.
export default function ProviderCards({
	providers,
	connections,
	signIns = [],
	cost,
	onChanged,
}: {
	providers: ProviderView[];
	connections: Connection[];
	signIns?: ProviderSignIn[];
	cost: CostSummary | null;
	onChanged(): void;
}): ReactNode {
	const t = useT();

	return (
		<section aria-labelledby="providers-title">
			<h2 id="providers-title" className="text-lg font-semibold">
				{t("ui.providers.title")}
			</h2>
			<p className="mt-1 text-sm text-muted-foreground">
				{t("ui.providers.intro")}
			</p>
			<ul className="mt-4 flex flex-col gap-4">
				{providers.map((provider) => (
					<ProviderCard
						key={provider.id}
						provider={provider}
						connections={connections}
						signIns={signIns}
						cost={cost}
						onChanged={onChanged}
					/>
				))}
			</ul>
		</section>
	);
}
