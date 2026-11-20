import { Head, Link, router } from "@inertiajs/react";
import { type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { ProviderSignIn } from "../../editor/api/types";

type Phase = "working" | "done" | "failed";

// The redirect address of the sign-in to an AI provider (SDD-016 section 12): the provider sends the
// person here with a state and a code (and, for the first sign-in with ChatGPT, the client it issued);
// the page hands them to the service with the session of the browser, and the service checks and finishes
// the sign-in. The page decides nothing, and it removes what the address held as soon as it is spent.
export default function SignInCallback(): ReactNode {
	const t = useT();
	const [phase, setPhase] = useState<Phase>("working");
	const [signIn, setSignIn] = useState<ProviderSignIn | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

	useEffect(() => {
		const query = new URLSearchParams(window.location.search);
		const state = query.get("state") ?? "";
		const code = query.get("code") ?? undefined;
		const error = query.get("error") ?? undefined;
		const clientId = query.get("client_id") ?? undefined;
		// The state and the code leave the address before the exchange starts, not after it ends (SDD-007
		// T-76). Inertia keeps the address of the page in its own history entry and writes it after the first
		// render, so the address is replaced through Inertia, which also drops the code from that entry.
		router.replace({
			url: window.location.pathname,
			preserveState: true,
			preserveScroll: true,
		});
		createClient(new HttpTransport("/api/v1"))
			.completeProviderSignIn({
				state,
				...(code === undefined ? {} : { code }),
				...(error === undefined ? {} : { error }),
				...(clientId === undefined ? {} : { client_id: clientId }),
			})
			.then((result) => {
				setSignIn(result);
				setPhase("done");
			})
			.catch((e) => {
				setFailure(failureOf(e));
				setPhase("failed");
			});
	}, []);

	return (
		<>
			<Head title={t("ui.providers.callback_title")} />
			<div className="mx-auto flex max-w-xl flex-col gap-4">
				<h1 className="text-2xl font-semibold">
					{t("ui.providers.callback_title")}
				</h1>
				{phase === "working" && (
					<p role="status" className="text-sm">
						{t("ui.providers.callback_working")}
					</p>
				)}
				{phase === "done" && signIn !== null && (
					<p role="status" className="text-sm" data-signed-in="">
						{t("ui.providers.callback_done")}{" "}
						<UntrustedText>{signIn.account ?? signIn.label}</UntrustedText>
					</p>
				)}
				{phase === "failed" && (
					<>
						<p className="text-sm">{t("ui.providers.callback_failed")}</p>
						<FailureNote failure={failure} />
					</>
				)}
				{phase !== "working" && (
					<p className="text-sm">
						<Link href="/connections" className="underline underline-offset-4">
							{t("ui.providers.back")}
						</Link>
					</p>
				)}
			</div>
		</>
	);
}
