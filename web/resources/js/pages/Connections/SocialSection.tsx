import { type ReactNode, useCallback, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { SocialAccount, SocialNetwork } from "../../editor/api/types";
import { PublishDialog } from "./PublishDialog";
import { SetupGuide } from "./SetupGuide";

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

interface Guide {
	network: SocialNetwork;
	connectionId?: string;
}

export function AccountRow({
	account,
	networkName,
	onSignInAgain,
	onPublish,
}: {
	account: SocialAccount;
	networkName: string;
	onSignInAgain(): void;
	onPublish(): void;
}): ReactNode {
	const t = useT();
	const ready = account.state === "ready";
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
					) : (
						t("ui.social.needs_sign_in")
					)}
				</p>
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
						/>
					))}
				</ul>
			)}
			{publishFor !== null && (
				<PublishDialog
					account={publishFor}
					onClose={() => setPublishFor(null)}
				/>
			)}
		</section>
	);
}
