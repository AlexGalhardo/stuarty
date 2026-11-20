import {
	type ReactNode,
	useEffect,
	useState,
	useSyncExternalStore,
} from "react";

import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
	type BackgroundAnswer,
	type BackgroundSettings,
	canBackground,
	getDesktopBridge,
	readBackground,
	subscribeDesktopBridge,
	writeBackground,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";

// The background mode of the desktop shell (SDD-012 section 21.3 item 7): keep running behind a tray icon
// when the window closes with a post scheduled, and start hidden at sign-in. It is drawn only in the shell
// (feature-detected); the shell keeps the choice and carries it out, the page shows and sends it, and
// says plainly what the mode cannot do.
export default function Background(): ReactNode {
	const t = useT();
	const bridge = useSyncExternalStore(
		subscribeDesktopBridge,
		getDesktopBridge,
		() => null,
	);
	const [settings, setSettings] = useState<BackgroundAnswer | null>(null);
	const [failed, setFailed] = useState<string | null>(null);

	useEffect(() => {
		if (!canBackground(bridge)) return;
		let live = true;
		readBackground(bridge).then(
			(answer) => {
				if (live) setSettings(answer);
			},
			() => undefined,
		);
		return () => {
			live = false;
		};
	}, [bridge]);

	if (!canBackground(bridge) || settings === null) return null;

	async function change(next: BackgroundSettings): Promise<void> {
		setFailed(null);
		try {
			const answer = await writeBackground(bridge, next);
			if (answer === null) {
				setFailed(t("ui.settings.background.failed"));
				return;
			}
			setSettings(answer);
			if (answer.error !== undefined) setFailed(answer.error);
		} catch {
			setFailed(t("ui.settings.background.failed"));
		}
	}

	return (
		<section
			aria-labelledby="settings-background"
			className="flex flex-col gap-3"
			data-testid="settings-background"
		>
			<h2 id="settings-background" className="text-lg font-semibold">
				{t("ui.settings.background.title")}
			</h2>
			<p className="text-sm text-muted-foreground">
				{t("ui.settings.background.intro")}
			</p>
			<div className="flex items-center justify-between gap-4">
				<Label htmlFor="background-keep">
					{t("ui.settings.background.keep_running")}
				</Label>
				<Switch
					id="background-keep"
					checked={settings.keep_running}
					onCheckedChange={(checked) =>
						void change({
							keep_running: checked,
							start_at_sign_in: checked && settings.start_at_sign_in,
							asked: true,
						})
					}
				/>
			</div>
			<div className="flex flex-col gap-1">
				<div className="flex items-center justify-between gap-4">
					<Label htmlFor="background-start">
						{t("ui.settings.background.start_at_sign_in")}
					</Label>
					<Switch
						id="background-start"
						checked={settings.start_at_sign_in}
						disabled={!settings.keep_running}
						onCheckedChange={(checked) =>
							void change({
								keep_running: settings.keep_running,
								start_at_sign_in: checked,
								asked: true,
							})
						}
					/>
				</div>
				<p className="text-sm text-muted-foreground">
					{t("ui.settings.background.start_at_sign_in_hint")}
				</p>
			</div>
			{failed !== null && (
				<p role="alert" className="text-sm text-destructive">
					{failed}
				</p>
			)}
			<ul className="list-disc pl-5 text-sm text-muted-foreground">
				<li>{t("ui.settings.background.limit_computer")}</li>
				<li>{t("ui.settings.background.limit_missed")}</li>
				<li>{t("ui.settings.background.limit_quit")}</li>
			</ul>
		</section>
	);
}
