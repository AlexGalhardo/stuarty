import { Head, Link } from "@inertiajs/react";
import { type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { SocialAccount } from "../../editor/api/types";

type Phase = "working" | "done" | "failed";

// The redirect address of the sign-in to a social network (SDD-020 sections 6 and 13): the network sends
// the person here with a state and a code; the page hands them to the service with the session of the
// browser, and the service checks and finishes the sign-in. The page decides nothing.
export default function SocialCallback(): ReactNode {
	const t = useT();
	const [phase, setPhase] = useState<Phase>("working");
	const [account, setAccount] = useState<SocialAccount | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

	useEffect(() => {
		const query = new URLSearchParams(window.location.search);
		const state = query.get("state") ?? "";
		const code = query.get("code") ?? undefined;
		const error = query.get("error") ?? undefined;
		createClient(new HttpTransport("/api/v1"))
			.completeSocial({
				state,
				...(code === undefined ? {} : { code }),
				...(error === undefined ? {} : { error }),
			})
			.then((result) => {
				setAccount(result);
				setPhase("done");
			})
			.catch((e) => {
				setFailure(failureOf(e));
				setPhase("failed");
			})
			.finally(() => {
				// The address held a state and a code, both spent now: do not leave them in the history.
				window.history.replaceState(
					window.history.state,
					"",
					window.location.pathname,
				);
			});
	}, []);

	return (
		<>
			<Head title={t("ui.social.callback_title")} />
			<div className="mx-auto flex max-w-xl flex-col gap-4">
				<h1 className="text-2xl font-semibold">
					{t("ui.social.callback_title")}
				</h1>
				{phase === "working" && (
					<p role="status" className="text-sm">
						{t("ui.social.callback_working")}
					</p>
				)}
				{phase === "done" &&
					account !== null &&
					(account.state === "needs_account" ? (
						<p role="status" className="text-sm" data-needs-account="">
							{t("ui.social.callback_choose")}
						</p>
					) : (
						<p role="status" className="text-sm" data-signed-in="">
							{t("ui.social.callback_done")}{" "}
							<UntrustedText>
								{account.account_title ?? account.label}
							</UntrustedText>
						</p>
					))}
				{phase === "failed" && (
					<>
						<p className="text-sm">{t("ui.social.callback_failed")}</p>
						<FailureNote failure={failure} />
					</>
				)}
				{phase !== "working" && (
					<p className="text-sm">
						<Link href="/connections" className="underline underline-offset-4">
							{t("ui.social.back")}
						</Link>
					</p>
				)}
			</div>
		</>
	);
}
