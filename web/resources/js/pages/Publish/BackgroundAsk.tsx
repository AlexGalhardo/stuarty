import { type ReactNode, useState, useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import {
	type BackgroundAnswer,
	canBackground,
	getDesktopBridge,
	readBackground,
	subscribeDesktopBridge,
	writeBackground,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";

// The first time a person schedules a post in the desktop shell, a dialog explains the background mode and
// records the answer, once (SDD-012 section 21.3 item 7). Nothing resident is installed silently: "Not now"
// records that it was asked and changes nothing else. In a browser there is no shell and nothing is asked.
export function useBackgroundAsk(): {
	ask: () => Promise<void>;
	dialog: ReactNode;
} {
	const t = useT();
	const bridge = useSyncExternalStore(
		subscribeDesktopBridge,
		getDesktopBridge,
		() => null,
	);
	const [current, setCurrent] = useState<BackgroundAnswer | null>(null);
	const [failed, setFailed] = useState(false);

	async function ask(): Promise<void> {
		if (!canBackground(bridge)) return;
		try {
			const settings = await readBackground(bridge);
			if (settings !== null && !settings.asked) setCurrent(settings);
		} catch {
			// The shell did not answer: nothing is asked, the post is scheduled all the same.
		}
	}

	async function answer(keep: boolean): Promise<void> {
		if (current === null) return;
		setFailed(false);
		try {
			const saved = await writeBackground(bridge, {
				keep_running: keep,
				start_at_sign_in: false,
				asked: true,
			});
			if (saved === null) {
				setFailed(true);
				return;
			}
			setCurrent(null);
		} catch {
			setFailed(true);
		}
	}

	const dialog = (
		<Dialog
			open={current !== null}
			onOpenChange={(open) => {
				if (!open) void answer(false);
			}}
		>
			<DialogContent data-testid="background-ask">
				<DialogHeader>
					<DialogTitle>{t("ui.settings.background.ask_title")}</DialogTitle>
					<DialogDescription>
						{t("ui.settings.background.intro")}
					</DialogDescription>
				</DialogHeader>
				<ul className="list-disc pl-5 text-sm text-muted-foreground">
					<li>{t("ui.settings.background.limit_computer")}</li>
					<li>{t("ui.settings.background.limit_missed")}</li>
					<li>{t("ui.settings.background.limit_quit")}</li>
				</ul>
				<p className="text-sm text-muted-foreground">
					{t("ui.settings.background.ask_later")}
				</p>
				{failed && (
					<p role="alert" className="text-sm text-destructive">
						{t("ui.settings.background.failed")}
					</p>
				)}
				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						data-testid="background-ask-no"
						onClick={() => void answer(false)}
					>
						{t("ui.settings.background.ask_no")}
					</Button>
					<Button
						type="button"
						data-testid="background-ask-yes"
						onClick={() => void answer(true)}
					>
						{t("ui.settings.background.ask_yes")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);

	return { ask, dialog };
}
