import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { SocialNetwork } from "../../editor/api/types";

const STEPS = [
	"project",
	"api",
	"consent",
	"client",
	"redirect",
	"paste",
] as const;

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// The guide to connect a social network with the person's own developer project (SDD-020 section 8,
// ADR-0035): the steps with the exact values to paste, what the research found and when, and the form
// that sends the client of the project. It decides nothing: the redirect address is the origin of this
// page and the path the service gave, and the address of the consent screen is the service's answer.
export function SetupGuide({
	network,
	redirectPath,
	connectionId,
	origin,
	onClose,
	navigate,
}: {
	network: SocialNetwork;
	redirectPath: string;
	connectionId?: string;
	origin: string;
	onClose(): void;
	navigate(address: string): void;
}): ReactNode {
	const t = useT();
	const { privileged } = useSession();
	const [clientId, setClientId] = useState("");
	const [clientSecret, setClientSecret] = useState("");
	const [label, setLabel] = useState("");
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const redirect = `${origin}${redirectPath}`;
	const again = connectionId !== undefined;
	const specific = t(`ui.social.${network.network}.step.project`);
	const prefix =
		specific === `ui.social.${network.network}.step.project`
			? "ui.social.generic"
			: `ui.social.${network.network}`;

	async function connect(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			const result = await api().connectSocial({
				network: network.network,
				redirect_uri: redirect,
				...(again
					? { connection_id: connectionId }
					: {
							client_id: clientId.trim(),
							client_secret: clientSecret,
							...(label.trim() === "" ? {} : { label: label.trim() }),
						}),
			});
			navigate(result.authorize_url);
		} catch (error) {
			setFailure(failureOf(error));
			setBusy(false);
		}
	}

	return (
		<section
			aria-labelledby="social-guide-title"
			className="mt-4 flex flex-col gap-4 rounded-xl border bg-card p-5 text-card-foreground"
		>
			<h3 id="social-guide-title" className="text-base font-semibold">
				{again
					? t("ui.social.guide_again_title", { name: network.name })
					: t("ui.social.guide_title", { name: network.name })}
			</h3>
			{!again && (
				<>
					<p className="text-sm text-muted-foreground">
						{t("ui.social.guide_intro", { name: network.name })}
					</p>
					<ol className="list-decimal space-y-2 pl-5 text-sm">
						{STEPS.map((step) => (
							<li key={step}>
								{t(`${prefix}.step.${step}`, {
									redirect,
									scopes: network.scopes.join(", "),
								})}
							</li>
						))}
					</ol>
					<div>
						<p className="text-sm font-medium">
							{t("ui.social.redirect_label")}
						</p>
						<code
							className="mt-1 block break-all rounded-lg bg-muted p-2 text-xs"
							data-redirect-address=""
						>
							{redirect}
						</code>
					</div>
					<p className="rounded-lg border p-3 text-sm" data-research-note="">
						{t(`${prefix}.research`, {
							read: network.guide_read,
						})}{" "}
						<a
							href={network.guide}
							target="_blank"
							rel="noreferrer noopener"
							className="underline underline-offset-4"
						>
							{t("ui.social.official_page")}
						</a>
					</p>
				</>
			)}
			<form
				onSubmit={(event) => void connect(event)}
				className="flex flex-col gap-4"
			>
				{!again && (
					<>
						<div className="flex flex-col gap-2">
							<Label htmlFor="social-client-id">
								{t("ui.social.client_id")}
							</Label>
							<Input
								id="social-client-id"
								required
								aria-invalid={failure === null ? undefined : true}
								aria-describedby={failure === null ? undefined : "setup-error"}
								autoComplete="off"
								value={clientId}
								onChange={(event) => setClientId(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="social-client-secret">
								{t("ui.social.client_secret")}
							</Label>
							<Input
								id="social-client-secret"
								required
								aria-invalid={failure === null ? undefined : true}
								aria-describedby={failure === null ? undefined : "setup-error"}
								type="password"
								autoComplete="off"
								value={clientSecret}
								onChange={(event) => setClientSecret(event.target.value)}
							/>
							<p className="text-xs text-muted-foreground">
								{t("ui.connections.secret_hint")}
							</p>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="social-label">
								{t("ui.social.account_label")}
							</Label>
							<Input
								id="social-label"
								maxLength={60}
								autoComplete="off"
								value={label}
								onChange={(event) => setLabel(event.target.value)}
							/>
						</div>
					</>
				)}
				<FailureNote id="setup-error" failure={failure} />
				<PersonOnlyNote id="social-person-only" />
				<div className="flex gap-2">
					<Button
						type="submit"
						disabled={
							!privileged ||
							busy ||
							(!again && (clientId.trim() === "" || clientSecret === ""))
						}
						aria-describedby={privileged ? undefined : "social-person-only"}
					>
						{again
							? t("ui.social.sign_in_again")
							: t("ui.social.start_sign_in")}
					</Button>
					<Button type="button" variant="outline" onClick={onClose}>
						{t("ui.social.close")}
					</Button>
				</div>
			</form>
		</section>
	);
}
