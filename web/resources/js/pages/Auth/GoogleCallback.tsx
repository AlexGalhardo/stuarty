import { Head, Link, router } from "@inertiajs/react";
import { type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { useT } from "@/lib/i18n";
import { safeNext } from "@/lib/safeNext";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";

// Where the person was going when they pressed "Sign in with Google" (the login page keeps it).
export const GOOGLE_NEXT_KEY = "stuarty.google.next";

type Phase = "working" | "done" | "failed";

// The redirect page of the Google sign-in (SDD-007 section 4.7, SDD-014 section 12.4): Google sends the
// person here with a state and a code; the page hands them to POST /auth/google/complete and goes on. It
// decides nothing: the server checks the state, the token and the link.
export default function GoogleCallback(): ReactNode {
	const t = useT();
	const [phase, setPhase] = useState<Phase>("working");
	const [failure, setFailure] = useState<Failure | null>(null);

	useEffect(() => {
		const query = new URLSearchParams(window.location.search);
		const code = query.get("code") ?? undefined;
		const error = query.get("error") ?? undefined;
		// The state and the code leave the address before the exchange starts, not after it ends (SDD-007
		// T-76). Inertia keeps the address of the page in its own history entry and writes it after the first
		// render, so the address is replaced through Inertia, which also drops the code from that entry.
		router.replace({
			url: window.location.pathname,
			preserveState: true,
			preserveScroll: true,
		});
		createClient(new HttpTransport(""))
			.googleComplete({
				state: query.get("state") ?? "",
				...(code === undefined ? {} : { code }),
				...(error === undefined ? {} : { error }),
			})
			.then((result) => {
				const next = window.sessionStorage.getItem(GOOGLE_NEXT_KEY);
				window.sessionStorage.removeItem(GOOGLE_NEXT_KEY);
				setPhase("done");
				// A document navigation, as after the password login (SDD-012 section 2.2).
				window.location.assign(
					result.user !== undefined ? safeNext(next) : "/settings/google",
				);
			})
			.catch((e) => {
				setFailure(failureOf(e));
				setPhase("failed");
			});
	}, []);

	return (
		<>
			<Head title={t("ui.google.callback_title")} />
			<div className="mx-auto flex max-w-xl flex-col gap-4">
				<h1 className="text-2xl font-semibold">
					{t("ui.google.callback_title")}
				</h1>
				{phase !== "failed" && (
					<p role="status" className="text-sm" data-google-phase={phase}>
						{t("ui.google.callback_working")}
					</p>
				)}
				{phase === "failed" && (
					<>
						<p className="text-sm" data-google-phase="failed">
							{t("ui.google.callback_failed")}
						</p>
						<FailureNote failure={failure} />
						<p className="text-sm">
							<Link href="/login" className="underline underline-offset-4">
								{t("ui.google.back_to_login")}
							</Link>
						</p>
					</>
				)}
			</div>
		</>
	);
}
