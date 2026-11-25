import { Head, Link, usePage } from "@inertiajs/react";
import { Palette } from "lucide-react";
import { useEffect, useState, useSyncExternalStore } from "react";

import { DataList, DataRow } from "@/components/DataRow";
import LocaleSwitcher from "@/components/LocaleSwitcher";
import { PageHeader } from "@/components/PageHeader";
import ThemeToggle from "@/components/ThemeToggle";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
	canBackground,
	getDesktopBridge,
	subscribeDesktopBridge,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";
import type { SharedProps } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import {
	readTimeUnit,
	type TimeUnit,
	writeTimeUnit,
} from "../../editor/state/prefs";
import AccountSettings from "./Account";
import Background from "./Background";
import { SELECT_CLASS, SettingsSection } from "./SettingsSection";
import Transcription from "./Transcription";

type Loaded =
	| { state: "loading" }
	| { state: "failed" }
	| { state: "ready"; version: string; workspace: string | undefined };

// The settings page (SDD-012 section 2.1): a list of its sections, then, in
// one column, the version and the workspace folder as the service reports
// them (`GET /system`, the about card, first), the preferences (language,
// theme and the unit a bare number typed in a time field gets), the brand kit,
// the account and the helpers of this computer. The list follows the same
// order and links to the sections that are on the page.
export default function SettingsIndex() {
	const t = useT();
	const account = usePage<SharedProps>().props.account;
	const bridge = useSyncExternalStore(
		subscribeDesktopBridge,
		getDesktopBridge,
		() => null,
	);
	const [system, setSystem] = useState<Loaded>({ state: "loading" });
	const [unit, setUnit] = useState<TimeUnit>("s");

	useEffect(() => {
		setUnit(readTimeUnit());
		let live = true;
		createClient(new HttpTransport("/api/v1"))
			.getSystem()
			.then((result) => {
				if (live)
					setSystem({
						state: "ready",
						version: result.version,
						workspace: result.workspace,
					});
			})
			.catch(() => {
				if (live) setSystem({ state: "failed" });
			});
		return () => {
			live = false;
		};
	}, []);

	const sections = [
		{ id: "settings-about", label: t("ui.settings.about") },
		{ id: "settings-preferences", label: t("ui.settings.preferences") },
		{ id: "settings-brand", label: t("ui.brand.title") },
		...(account?.state === "unlocked"
			? [{ id: "settings-account", label: t("ui.account.settings.title") }]
			: []),
		...(canBackground(bridge)
			? [
					{
						id: "settings-background",
						label: t("ui.settings.background.title"),
					},
				]
			: []),
		{
			id: "settings-transcription",
			label: t("ui.settings.transcription.title"),
		},
	];

	return (
		<>
			<Head title={t("ui.settings.title")} />
			<div className="mx-auto flex max-w-6xl flex-col gap-6">
				<PageHeader
					title={t("ui.settings.title")}
					description={t("ui.settings.intro")}
				/>
				<div className="grid gap-6 lg:grid-cols-[12rem_minmax(0,1fr)]">
					<nav
						aria-label={t("ui.settings.sections")}
						className="flex flex-wrap gap-1 lg:flex-col lg:flex-nowrap lg:self-start"
					>
						{sections.map((section) => (
							<a
								key={section.id}
								href={`#${section.id}`}
								className="rounded-lg px-3 py-2 text-sm font-medium whitespace-nowrap text-text-muted hover:bg-surface-hover hover:text-text max-md:py-3"
							>
								{section.label}
							</a>
						))}
					</nav>
					<div className="flex min-w-0 flex-col gap-4">
						<SettingsSection id="settings-about" title={t("ui.settings.about")}>
							{system.state === "loading" && (
								<p className="text-sm text-text-muted">
									{t("ui.settings.loading")}
								</p>
							)}
							{system.state === "failed" && (
								<p role="alert" className="text-sm text-danger-text">
									{t("ui.settings.unavailable")}
								</p>
							)}
							{system.state === "ready" && (
								<DataList>
									<DataRow label={t("ui.settings.version")}>
										<span data-testid="settings-version">{system.version}</span>
									</DataRow>
									<DataRow
										label={t("ui.settings.workspace")}
										className="grid-cols-1 [&>dd]:col-start-1 [&>dd]:row-span-1 [&>dd]:row-start-2 [&>dd]:justify-self-start [&>dd]:text-left"
									>
										<span
											data-testid="settings-workspace"
											className="[overflow-wrap:anywhere]"
										>
											{system.workspace ?? t("ui.settings.workspace_none")}
										</span>
									</DataRow>
								</DataList>
							)}
						</SettingsSection>
						<SettingsSection
							id="settings-preferences"
							title={t("ui.settings.preferences")}
						>
							<div className="flex flex-wrap items-center justify-between gap-4">
								<span className="text-sm font-medium">
									{t("ui.settings.language")}
								</span>
								<LocaleSwitcher />
							</div>
							<div className="flex flex-wrap items-center justify-between gap-4">
								<span className="text-sm font-medium">
									{t("ui.settings.theme")}
								</span>
								<ThemeToggle />
							</div>
							<div className="flex flex-col gap-1">
								<div className="flex flex-wrap items-center justify-between gap-4">
									<Label htmlFor="settings-time-unit">
										{t("ui.settings.time_unit")}
									</Label>
									<select
										id="settings-time-unit"
										value={unit}
										className={SELECT_CLASS}
										onChange={(event) => {
											const next = event.target.value === "f" ? "f" : "s";
											writeTimeUnit(next);
											setUnit(next);
										}}
									>
										<option value="s">
											{t("ui.settings.time_unit_seconds")}
										</option>
										<option value="f">
											{t("ui.settings.time_unit_frames")}
										</option>
									</select>
								</div>
								<p className="text-sm text-text-subtle">
									{t("ui.settings.time_unit_hint")}
								</p>
							</div>
						</SettingsSection>
						<SettingsSection
							id="settings-brand"
							title={t("ui.brand.title")}
							description={t("ui.brand.intro")}
						>
							<div>
								<Button asChild variant="outline">
									<Link href="/settings/brand-kit">
										<Palette aria-hidden="true" />
										{t("ui.brand.open")}
									</Link>
								</Button>
							</div>
						</SettingsSection>
						<AccountSettings />
						<Background />
						<Transcription />
					</div>
				</div>
			</div>
		</>
	);
}
