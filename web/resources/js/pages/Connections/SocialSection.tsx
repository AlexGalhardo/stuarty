import { type ReactNode, useCallback, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale, useT } from "@/lib/i18n";
import {
	formatMoney,
	microsToInput,
	parseMicros,
} from "../../dashboard/approval/money";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	SocialAccount,
	SocialChoice,
	SocialNetwork,
} from "../../editor/api/types";
import { PublishDialog } from "./PublishDialog";
import { SetupGuide } from "./SetupGuide";

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

interface Guide {
	network: SocialNetwork;
	connectionId?: string;
}

// The accounts a sign-in reached (a Facebook Page, an Instagram professional account): the person chooses
// which one the connection publishes to, and nothing is chosen for them (SDD-020 section 17). The titles are
// text from the network. The page only sends the choice; the service checks it.
export function AccountPicker({
	connectionId,
	choices,
	busy,
	onSelect,
}: {
	connectionId: string;
	choices: SocialChoice[];
	busy: boolean;
	onSelect(accountId: string): void;
}): ReactNode {
	const t = useT();
	const [picked, setPicked] = useState("");
	return (
		<fieldset
			className="mt-3 flex flex-col gap-2"
			data-account-picker={connectionId}
		>
			<legend className="text-sm font-medium">
				{t("ui.social.pick_title")}
			</legend>
			<p className="text-xs text-muted-foreground">
				{t("ui.social.pick_hint")}
			</p>
			{choices.map((choice) => (
				<label key={choice.id} className="flex items-center gap-2 text-sm">
					<input
						type="radio"
						name={`social-account-${connectionId}`}
						className="accent-primary"
						value={choice.id}
						checked={picked === choice.id}
						onChange={() => setPicked(choice.id)}
					/>
					<UntrustedText>{choice.title}</UntrustedText>
				</label>
			))}
			<div>
				<Button
					type="button"
					disabled={busy || picked === ""}
					onClick={() => onSelect(picked)}
				>
					{t("ui.social.pick_use")}
				</Button>
			</div>
		</fieldset>
	);
}

// The price of a post on a network that charges for one (X): the amounts the approval will show, with the date
// they were read or edited, and a form to correct them when the network changes them (SDD-020 section 39). The
// page sends what is typed; the service checks it and keeps it.
export function PriceEditor({
	account,
	busy,
	onSave,
}: {
	account: SocialAccount;
	busy: boolean;
	onSave(postMicros: number, linkMicros: number): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const price = account.price;
	const [post, setPost] = useState(microsToInput(price?.post_micros ?? 0));
	const [link, setLink] = useState(microsToInput(price?.link_micros ?? 0));
	if (price === undefined) return null;
	const postMicros = parseMicros(post);
	const linkMicros = parseMicros(link);
	return (
		<fieldset
			className="mt-3 flex flex-col gap-2"
			data-price-editor={account.connection_id}
		>
			<legend className="text-sm font-medium">
				{t("ui.social.price_title")}
			</legend>
			<p className="text-xs text-muted-foreground" data-testid="account-price">
				{t("ui.social.price_now", {
					post: formatMoney(
						{ micros: price.post_micros, currency: price.currency },
						locale,
					),
					link: formatMoney(
						{ micros: price.link_micros, currency: price.currency },
						locale,
					),
					date: price.edited ?? price.read,
				})}
			</p>
			<div className="flex flex-wrap items-end gap-2">
				<div className="flex flex-col gap-1">
					<Label htmlFor={`price-post-${account.connection_id}`}>
						{t("ui.social.price_post", { currency: price.currency })}
					</Label>
					<Input
						id={`price-post-${account.connection_id}`}
						inputMode="decimal"
						className="w-28"
						value={post}
						onChange={(event) => setPost(event.target.value)}
					/>
				</div>
				<div className="flex flex-col gap-1">
					<Label htmlFor={`price-link-${account.connection_id}`}>
						{t("ui.social.price_link", { currency: price.currency })}
					</Label>
					<Input
						id={`price-link-${account.connection_id}`}
						inputMode="decimal"
						className="w-28"
						value={link}
						onChange={(event) => setLink(event.target.value)}
					/>
				</div>
				<Button
					type="button"
					variant="outline"
					disabled={busy || postMicros === null || linkMicros === null}
					onClick={() =>
						postMicros !== null &&
						linkMicros !== null &&
						onSave(postMicros, linkMicros)
					}
				>
					{t("ui.social.price_save")}
				</Button>
			</div>
		</fieldset>
	);
}

export function AccountRow({
	account,
	networkName,
	onSignInAgain,
	onPublish,
	onSelect,
	onPrice,
	busy = false,
}: {
	account: SocialAccount;
	networkName: string;
	onSignInAgain(): void;
	onPublish(): void;
	onSelect?(accountId: string): void;
	onPrice?(postMicros: number, linkMicros: number): void;
	busy?: boolean;
}): ReactNode {
	const t = useT();
	const ready = account.state === "ready";
	const choosing =
		account.state === "needs_account" &&
		onSelect !== undefined &&
		(account.choices ?? []).length > 0;
	return (
		<li
			className="flex items-start justify-between gap-4 p-4"
			data-social-account={account.connection_id}
		>
			<div className="min-w-0">
				<p className="truncate font-medium">
					{networkName} · <UntrustedText>{account.label}</UntrustedText>
				</p>
				<p className="text-xs text-muted-foreground">
					{ready ? (
						<>
							{t("ui.social.signed_in_as")}{" "}
							<UntrustedText>{account.account_title ?? ""}</UntrustedText>
						</>
					) : account.state === "needs_account" ? (
						t("ui.social.needs_account")
					) : (
						t("ui.social.needs_sign_in")
					)}
				</p>
				{ready && onPrice !== undefined && account.price !== undefined && (
					<PriceEditor account={account} busy={busy} onSave={onPrice} />
				)}
				{choosing && (
					<AccountPicker
						connectionId={account.connection_id}
						choices={account.choices ?? []}
						busy={busy}
						onSelect={onSelect}
					/>
				)}
			</div>
			<div className="flex shrink-0 gap-2">
				<Button type="button" variant="outline" onClick={onSignInAgain}>
					{t("ui.social.sign_in_again")}
					<span className="sr-only">
						{" "}
						<UntrustedText>{account.label}</UntrustedText>
					</span>
				</Button>
				<Button type="button" disabled={!ready} onClick={onPublish}>
					{t("ui.social.publish_now")}
					<span className="sr-only">
						{" "}
						<UntrustedText>{account.label}</UntrustedText>
					</span>
				</Button>
			</div>
		</li>
	);
}

// The social networks of the Connections page (SDD-020 section 8): connect a network with your own
// developer app, see which accounts are signed in, and send a finished video to one of them. It shows
// what the service answers and decides nothing.
export default function SocialSection(): ReactNode {
	const t = useT();
	const [networks, setNetworks] = useState<SocialNetwork[] | null>(null);
	const [redirectPath, setRedirectPath] = useState("/social/callback");
	const [accounts, setAccounts] = useState<SocialAccount[]>([]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [guide, setGuide] = useState<Guide | null>(null);
	const [publishFor, setPublishFor] = useState<SocialAccount | null>(null);
	const [choosing, setChoosing] = useState(false);

	const load = useCallback(async (): Promise<void> => {
		try {
			const [n, a] = await Promise.all([
				api().listSocialNetworks(),
				api().listSocialAccounts(),
			]);
			setNetworks(n.networks);
			setRedirectPath(n.redirect_path);
			setAccounts(a.accounts);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}, []);

	useEffect(() => {
		void load();
	}, [load]);

	async function choose(account: SocialAccount, id: string): Promise<void> {
		setChoosing(true);
		setFailure(null);
		try {
			await api().selectSocialAccount({
				connection_id: account.connection_id,
				account_id: id,
			});
			await load();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setChoosing(false);
		}
	}

	async function setPrice(
		account: SocialAccount,
		postMicros: number,
		linkMicros: number,
	): Promise<void> {
		setChoosing(true);
		setFailure(null);
		try {
			await api().setSocialPrice({
				connection_id: account.connection_id,
				post_micros: postMicros,
				link_micros: linkMicros,
			});
			await load();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setChoosing(false);
		}
	}

	if (networks === null || networks.length === 0) {
		return failure === null ? null : <FailureNote failure={failure} />;
	}

	const nameOf = (id: string): string =>
		networks.find((n) => n.network === id)?.name ?? id;

	return (
		<section aria-labelledby="social-title">
			<h2 id="social-title" className="text-lg font-semibold">
				{t("ui.social.title")}
			</h2>
			<p className="mt-1 text-sm text-muted-foreground">
				{t("ui.social.intro")}
			</p>
			<FailureNote failure={failure} />
			<div className="mt-4 flex flex-wrap gap-2">
				{networks.map((network) => (
					<Button
						key={network.network}
						type="button"
						variant="outline"
						onClick={() => setGuide({ network })}
					>
						{t("ui.social.connect", { name: network.name })}
					</Button>
				))}
			</div>
			{guide !== null && (
				<SetupGuide
					network={guide.network}
					redirectPath={redirectPath}
					connectionId={guide.connectionId}
					origin={window.location.origin}
					onClose={() => setGuide(null)}
					navigate={(address) => window.location.assign(address)}
				/>
			)}
			{accounts.length > 0 && (
				<ul className="mt-4 divide-y rounded-xl border bg-card text-card-foreground">
					{accounts.map((account) => (
						<AccountRow
							key={account.connection_id}
							account={account}
							networkName={nameOf(account.network)}
							onSignInAgain={() => {
								const network = networks.find(
									(n) => n.network === account.network,
								);
								if (network !== undefined) {
									setGuide({ network, connectionId: account.connection_id });
								}
							}}
							onPublish={() => setPublishFor(account)}
							onSelect={(id) => void choose(account, id)}
							onPrice={(post, link) => void setPrice(account, post, link)}
							busy={choosing}
						/>
					))}
				</ul>
			)}
			{publishFor !== null && (
				<PublishDialog
					account={publishFor}
					network={networks.find((n) => n.network === publishFor.network)}
					onClose={() => setPublishFor(null)}
				/>
			)}
		</section>
	);
}
