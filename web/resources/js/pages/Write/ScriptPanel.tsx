import { type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type { ScriptEstimateResult, WritingClient } from "./client";
import { keyed } from "./keyed";

// The pause between the last keystroke and the question to the service.
export const CHECK_DELAY_MS = 400;

// m:ss.s of a number of milliseconds the core gave: a way of writing it, not a result.
export function clock(ms: number): string {
	const tenths = Math.round(ms / 100);
	const minutes = Math.floor(tenths / 600);
	const seconds = (tenths % 600) / 10;
	return `${minutes}:${seconds.toFixed(1).padStart(4, "0")}`;
}

// The duration of a script when read aloud (SDD-019 sections 4 and 11). The text goes to the service and the
// estimate comes back: the person sets the pace, the core counts the words and the time.
export default function ScriptPanel({
	client,
	projectId,
	body,
}: {
	client: WritingClient;
	projectId: string;
	body: string;
}): ReactNode {
	const t = useT();
	const [pace, setPace] = useState("150");
	const [estimate, setEstimate] = useState<ScriptEstimateResult | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

	useEffect(() => {
		if (body.trim() === "") {
			setEstimate(null);
			setFailure(null);
			return;
		}
		let live = true;
		const timer = window.setTimeout(() => {
			const wpm = pace.trim() === "" ? undefined : Number(pace);
			client
				.estimateScript(body, projectId, wpm)
				.then((result) => {
					if (!live) return;
					setEstimate(result);
					setFailure(null);
				})
				.catch((error: unknown) => {
					if (!live) return;
					setEstimate(null);
					setFailure(failureOf(error));
				});
		}, CHECK_DELAY_MS);
		return () => {
			live = false;
			window.clearTimeout(timer);
		};
	}, [client, projectId, body, pace]);

	return (
		<section
			aria-labelledby="script-heading"
			className="flex flex-col gap-2"
			data-testid="script-panel"
		>
			<h3 id="script-heading" className="font-medium">
				{t("ui.write.script.heading")}
			</h3>
			<div className="flex items-center gap-2">
				<Label htmlFor="script-pace">{t("ui.write.script.pace")}</Label>
				<Input
					id="script-pace"
					type="number"
					inputMode="numeric"
					className="w-24"
					value={pace}
					onChange={(event) => setPace(event.target.value)}
				/>
			</div>
			<FailureNote failure={failure} />
			<div
				role="status"
				aria-live="polite"
				className="flex flex-col gap-1 text-sm"
			>
				{estimate === null && failure === null && (
					<p className="text-muted-foreground">{t("ui.write.script.empty")}</p>
				)}
				{estimate !== null && (
					<>
						<p data-testid="script-duration">
							{t("ui.write.script.result", {
								duration: clock(estimate.duration_ms),
								pace: String(estimate.words_per_minute),
							})}
						</p>
						<p>
							{t("ui.write.script.words", {
								words: String(estimate.words),
								screen: String(estimate.on_screen_words),
							})}
						</p>
						{estimate.plain && (
							<p className="text-muted-foreground">
								{t("ui.write.script.plain")}
							</p>
						)}
					</>
				)}
			</div>
			{estimate !== null && estimate.scenes.length > 0 && (
				<>
					<h4 className="text-sm font-medium">{t("ui.write.script.scenes")}</h4>
					<ul className="flex flex-col gap-1 text-sm">
						{keyed(estimate.scenes, (s) => s.title).map(
							({ key, item: scene }) => (
								<li key={key}>
									<UntrustedText>
										{t("ui.write.script.scene_line", {
											title:
												scene.title === ""
													? t("ui.write.script.untitled")
													: scene.title,
											words: String(scene.words),
											duration: clock(scene.duration_ms),
										})}
									</UntrustedText>
								</li>
							),
						)}
					</ul>
				</>
			)}
		</section>
	);
}
