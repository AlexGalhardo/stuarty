import { Bot } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "@/components/ui/popover";
import { type Translate, useLocale, useT } from "@/lib/i18n";
import { announce } from "../a11y/announce";
import {
	ACTIVE_MS,
	type ActivityEntry,
	actorLabel,
} from "../state/activityStore";
import { useActivityState } from "../state/EditorContext";

const SETTLED_MS = 5 * 60 * 1000;
const ANNOUNCE_EVERY_MS = 2000;

function nowMs(): number {
	return performance.now();
}

function relative(locale: string, elapsedMs: number): string {
	const format = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
	const seconds = Math.max(0, Math.round(elapsedMs / 1000));
	if (seconds < 60) return format.format(-seconds, "second");
	return format.format(-Math.round(seconds / 60), "minute");
}

// "changed 3 items and 1 track": localized from the counts of the diff kinds.
function summary(t: Translate, counts: ActivityEntry["counts"]): string {
	const parts: string[] = [];
	if (counts.items > 0) {
		parts.push(t("ui.editor.activity.summary.items", { count: counts.items }));
	}
	if (counts.tracks > 0) {
		parts.push(
			t("ui.editor.activity.summary.tracks", { count: counts.tracks }),
		);
	}
	if (counts.assets > 0) {
		parts.push(
			t("ui.editor.activity.summary.assets", { count: counts.assets }),
		);
	}
	return parts.length > 0
		? parts.join(", ")
		: t("ui.editor.activity.summary.other");
}

// The activity indicator of SDD-012 sections 5.8 and 6.2: names the actor of
// the latest remote batch while it is fresh, then settles to "Last edit by".
export default function ActivityIndicator(): ReactNode {
	const t = useT();
	const locale = useLocale();
	const entries = useActivityState((s) => s.entries);
	const active = useActivityState((s) => s.active);
	const [now, setNow] = useState(nowMs);
	const announcedRevision = useRef<number | null>(null);
	const latest = entries[0];

	useEffect(() => {
		if (latest === undefined) return;
		const timer = setInterval(() => setNow(nowMs()), 1000);
		return () => clearInterval(timer);
	}, [latest]);

	// The live region (a11y/LiveRegion) announces a remote edit at most once
	// every 2 s (SDD-012 section 5.8), also while this button is on screen.
	useEffect(() => {
		if (latest === undefined || latest.revision === announcedRevision.current)
			return;
		announcedRevision.current = latest.revision;
		announce(
			"ui.editor.activity.announce",
			{ actor: actorLabel(latest.actor) },
			{ minIntervalMs: ANNOUNCE_EVERY_MS },
		);
	}, [latest]);

	if (latest === undefined || now - latest.at > SETTLED_MS) {
		return null;
	}

	const fresh = Object.entries(active).filter(([, at]) => now - at < ACTIVE_MS);
	const editing = now - latest.at < ACTIVE_MS;
	const others = fresh.filter(([actor]) => actor !== latest.actor).length;
	const actor = actorLabel(latest.actor);
	const text = editing
		? others > 0
			? t("ui.editor.activity.editing_others", { actor, count: others })
			: t("ui.editor.activity.editing", { actor })
		: t("ui.editor.activity.last", {
				actor,
				ago: relative(locale, now - latest.at),
			});

	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button type="button" variant="ghost" size="sm" className="gap-2">
					<span
						aria-hidden="true"
						className={`size-2 rounded-full ${editing ? "bg-agent" : "bg-muted-foreground"}`}
					/>
					<Bot aria-hidden="true" />
					<UntrustedText>{text}</UntrustedText>
				</Button>
			</PopoverTrigger>
			<PopoverContent align="end" className="w-80">
				<h2 className="text-sm font-medium">
					{t("ui.editor.activity.recent")}
				</h2>
				<ul className="flex flex-col gap-2">
					{entries.map((entry) => (
						<li key={`${entry.revision}-${entry.at}`} className="text-xs">
							<UntrustedText className="font-medium">
								{actorLabel(entry.actor)}
							</UntrustedText>{" "}
							{summary(t, entry.counts)}
							<span className="block text-muted-foreground">
								{t("ui.editor.activity.revision", {
									revision: entry.revision,
								})}
								, {relative(locale, now - entry.at)}
							</span>
						</li>
					))}
				</ul>
			</PopoverContent>
		</Popover>
	);
}
