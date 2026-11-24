import { Info } from "lucide-react";
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
import { SettingsSection } from "./SettingsSection";

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
	const [unread, setUnread] = useState(false);
	const [failed, setFailed] = useState<string | null>(null);

	useEffect(() => {
		if (!canBackground(bridge)) return;
		let live = true;
		readBackground(bridge).then(
			(answer) => {
				if (live) setSettings(answer);
			},
			() => {
				if (live) setUnread(true);
			},
		);
		return () => {
			live = false;
		};
	}, [bridge]);

	if (!canBackground(bridge)) return null;

	if (settings === null)
		return (
			<SettingsSection
				id="settings-background"
				title={t("ui.settings.background.title")}
				data-testid="settings-background-pending"
			>
				<p
					role={unread ? "alert" : "status"}
					className={
						unread ? "text-sm text-danger-text" : "text-sm text-text-muted"
					}
				>
					{unread ? t("ui.settings.unavailable") : t("ui.settings.loading")}
				</p>
			</SettingsSection>
		);

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

	const current = settings;
	return (
		<SettingsSection
			id="settings-background"
			title={t("ui.settings.background.title")}
			description={t("ui.settings.background.intro")}
			data-testid="settings-background"
		>
			<div className="flex items-center justify-between gap-4 border-b border-border pb-3">
				<Label htmlFor="background-keep">
					{t("ui.settings.background.keep_running")}
				</Label>
				<Switch
					id="background-keep"
					checked={current.keep_running}
					onCheckedChange={(checked) =>
						void change({
							keep_running: checked,
							start_at_sign_in: checked && current.start_at_sign_in,
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
						checked={current.start_at_sign_in}
						disabled={!current.keep_running}
						onCheckedChange={(checked) =>
							void change({
								keep_running: current.keep_running,
								start_at_sign_in: checked,
								asked: true,
							})
						}
					/>
				</div>
				<p className="text-sm text-text-subtle">
					{t("ui.settings.background.start_at_sign_in_hint")}
				</p>
			</div>
			{failed !== null && (
				<p role="alert" className="text-sm text-danger-text">
					{failed}
				</p>
			)}
			<ul className="flex flex-col gap-2 text-sm text-text-muted">
				{(["computer", "missed", "quit"] as const).map((limit) => (
					<li key={limit} className="flex items-start gap-2">
						<Info aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
						{t(`ui.settings.background.limit_${limit}`)}
					</li>
				))}
			</ul>
		</SettingsSection>
	);
}
