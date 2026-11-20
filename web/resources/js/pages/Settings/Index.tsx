import { Head, Link } from "@inertiajs/react";
import { useEffect, useState } from "react";

import LocaleSwitcher from "@/components/LocaleSwitcher";
import ThemeToggle from "@/components/ThemeToggle";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import {
	readTimeUnit,
	type TimeUnit,
	writeTimeUnit,
} from "../../editor/state/prefs";
import Background from "./Background";
import Transcription from "./Transcription";

type Loaded =
	| { state: "loading" }
	| { state: "failed" }
	| { state: "ready"; version: string; workspace: string | undefined };

// The settings page (SDD-012 section 2.1): language, theme and the unit a bare
// number typed in a time field gets, then the version and the workspace folder
// as the service reports them (`GET /system`).
export default function SettingsIndex() {
	const t = useT();
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

	return (
		<>
			<Head title={t("ui.settings.title")} />
			<div className="mx-auto flex max-w-2xl flex-col gap-8">
				<h1 className="text-2xl font-semibold">{t("ui.settings.title")}</h1>
				<section
					aria-labelledby="settings-preferences"
					className="flex flex-col gap-4"
				>
					<h2 id="settings-preferences" className="text-lg font-semibold">
						{t("ui.settings.preferences")}
					</h2>
					<div className="flex items-center justify-between gap-4">
						<span className="text-sm">{t("ui.settings.language")}</span>
						<LocaleSwitcher />
					</div>
					<div className="flex items-center justify-between gap-4">
						<span className="text-sm">{t("ui.settings.theme")}</span>
						<ThemeToggle />
					</div>
					<div className="flex flex-col gap-1">
						<div className="flex items-center justify-between gap-4">
							<Label htmlFor="settings-time-unit">
								{t("ui.settings.time_unit")}
							</Label>
							<select
								id="settings-time-unit"
								value={unit}
								className="h-8 rounded-lg border border-input bg-transparent px-2.5 text-sm dark:bg-input/30"
								onChange={(event) => {
									const next = event.target.value === "f" ? "f" : "s";
									writeTimeUnit(next);
									setUnit(next);
								}}
							>
								<option value="s">{t("ui.settings.time_unit_seconds")}</option>
								<option value="f">{t("ui.settings.time_unit_frames")}</option>
							</select>
						</div>
						<p className="text-sm text-muted-foreground">
							{t("ui.settings.time_unit_hint")}
						</p>
					</div>
				</section>
				<section
					aria-labelledby="settings-brand"
					className="flex flex-col gap-3"
				>
					<h2 id="settings-brand" className="text-lg font-semibold">
						{t("ui.brand.title")}
					</h2>
					<p className="text-sm text-muted-foreground">{t("ui.brand.intro")}</p>
					<div>
						<Button asChild variant="outline">
							<Link href="/settings/brand-kit">{t("ui.brand.open")}</Link>
						</Button>
					</div>
				</section>
				<Background />
				<Transcription />
				<section
					aria-labelledby="settings-about"
					className="flex flex-col gap-3"
				>
					<h2 id="settings-about" className="text-lg font-semibold">
						{t("ui.settings.about")}
					</h2>
					{system.state === "loading" && (
						<p className="text-sm text-muted-foreground">
							{t("ui.settings.loading")}
						</p>
					)}
					{system.state === "failed" && (
						<p role="alert" className="text-sm text-destructive">
							{t("ui.settings.unavailable")}
						</p>
					)}
					{system.state === "ready" && (
						<dl className="grid grid-cols-[10rem_1fr] gap-x-4 gap-y-2 text-sm">
							<dt className="text-muted-foreground">
								{t("ui.settings.version")}
							</dt>
							<dd data-testid="settings-version">{system.version}</dd>
							<dt className="text-muted-foreground">
								{t("ui.settings.workspace")}
							</dt>
							<dd data-testid="settings-workspace" className="break-all">
								{system.workspace ?? t("ui.settings.workspace_none")}
							</dd>
						</dl>
					)}
				</section>
			</div>
		</>
	);
}
