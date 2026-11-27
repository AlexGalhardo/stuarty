import { Download, RefreshCw } from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/cn";
import {
	canUpdate,
	checkDesktopUpdates,
	type DesktopBridge,
	type DesktopUpdateState,
	getDesktopBridge,
	installDesktopUpdate,
	readUpdateState,
	subscribeDesktopBridge,
	writeAutoUpdate,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";

function statusLine(
	t: ReturnType<typeof useT>,
	state: DesktopUpdateState,
): string | null {
	switch (state.status) {
		case "available":
			return state.available_version
				? t("ui.desktop_update.status.available", {
						version: state.available_version,
					})
				: null;
		case "up_to_date":
			return t("ui.desktop_update.status.up_to_date");
		case "checking":
			return t("ui.desktop_update.status.checking");
		case "downloading":
			return t("ui.desktop_update.status.downloading", {
				progress: String(state.progress ?? 0),
			});
		case "installing":
			return t("ui.desktop_update.status.installing");
		case "offline":
			return t("ui.desktop_update.status.offline");
		case "unconfigured":
			return t("ui.desktop_update.status.unconfigured");
		case "error":
			return t("ui.desktop_update.status.error");
		default:
			return null;
	}
}

// Desktop-only in-app updater control (epic 101): hidden in plain web browsers,
// shown on the Login screen and in the dashboard sidebar footer inside Stuarty Desktop.
export function DesktopUpdateControl({
	variant,
	collapsed = false,
	bridgeOverride,
	initialState,
}: {
	variant: "card" | "sidebar";
	collapsed?: boolean;
	// Optional injection for SSR/unit tests; browser runtime reads window.vukemDesktop.
	bridgeOverride?: DesktopBridge | null;
	initialState?: DesktopUpdateState | null;
}): ReactNode {
	const t = useT();
	const switchId = useId();
	const [bridge, setBridge] = useState<DesktopBridge | null>(() =>
		bridgeOverride !== undefined ? bridgeOverride : getDesktopBridge(),
	);
	const [state, setState] = useState<DesktopUpdateState | null>(
		initialState ?? null,
	);
	const [busy, setBusy] = useState(false);

	useEffect(() => {
		if (bridgeOverride !== undefined) {
			setBridge(bridgeOverride);
			return;
		}
		const sync = (): void => setBridge(getDesktopBridge());
		sync();
		return subscribeDesktopBridge(sync);
	}, [bridgeOverride]);

	const available = canUpdate(bridge);

	useEffect(() => {
		if (!available || bridge === null) return;
		let live = true;
		void readUpdateState(bridge)
			.then((next) => {
				if (live && next !== null) setState(next);
			})
			.catch(() => undefined);
		return () => {
			live = false;
		};
	}, [available, bridge]);

	if (!available) return null;

	const current: DesktopUpdateState = state ?? {
		current_version: "",
		auto_update: false,
		status: "idle",
	};

	const hasOffer =
		current.status === "available" &&
		current.available_version !== undefined &&
		current.available_version !== "";
	const working =
		busy ||
		current.status === "checking" ||
		current.status === "downloading" ||
		current.status === "installing";

	const buttonLabel = hasOffer
		? t("ui.desktop_update.install", {
				version: current.available_version ?? "",
			})
		: current.status === "downloading"
			? t("ui.desktop_update.status.downloading", {
					progress: String(current.progress ?? 0),
				})
			: t("ui.desktop_update.check");

	async function onAction(): Promise<void> {
		if (bridge === null || working) return;
		setBusy(true);
		try {
			const next = hasOffer
				? await installDesktopUpdate(bridge)
				: await checkDesktopUpdates(bridge);
			if (next !== null) setState(next);
		} catch {
			setState((prev) => ({
				current_version: prev?.current_version ?? "",
				auto_update: prev?.auto_update ?? false,
				status: "error",
			}));
		} finally {
			setBusy(false);
		}
	}

	async function onToggleAuto(checked: boolean): Promise<void> {
		if (bridge === null) return;
		setState((prev) => ({
			current_version: prev?.current_version ?? "",
			status: prev?.status ?? "idle",
			...prev,
			auto_update: checked,
		}));
		try {
			const next = await writeAutoUpdate(bridge, checked);
			if (next !== null) setState(next);
		} catch {
			setState((prev) => ({
				current_version: prev?.current_version ?? "",
				status: prev?.status ?? "error",
				...prev,
				auto_update: !checked,
			}));
		}
	}

	if (variant === "sidebar" && collapsed) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						type="button"
						variant={hasOffer ? "default" : "ghost"}
						size="icon"
						disabled={working}
						aria-label={buttonLabel}
						data-testid="desktop-update-action"
						onClick={() => void onAction()}
					>
						{hasOffer ? (
							<Download aria-hidden="true" />
						) : (
							<RefreshCw aria-hidden="true" />
						)}
					</Button>
				</TooltipTrigger>
				<TooltipContent side="right">{buttonLabel}</TooltipContent>
			</Tooltip>
		);
	}

	const note = statusLine(t, current);

	return (
		<section
			aria-label={t("ui.desktop_update.title")}
			data-testid={`desktop-update-${variant}`}
			className={cn(
				"flex flex-col gap-2 text-xs",
				variant === "card"
					? "mx-auto mt-4 w-full max-w-md rounded-lg border border-border bg-surface p-3"
					: "border-t border-border pt-2",
			)}
		>
			<Button
				type="button"
				variant={hasOffer ? "default" : "outline"}
				size="sm"
				disabled={working}
				data-testid="desktop-update-action"
				className="w-full justify-center"
				onClick={() => void onAction()}
			>
				{hasOffer ? (
					<Download aria-hidden="true" className="size-3.5 shrink-0" />
				) : (
					<RefreshCw aria-hidden="true" className="size-3.5 shrink-0" />
				)}
				<span className="truncate">{buttonLabel}</span>
			</Button>
			{note !== null && (
				<p
					role="status"
					data-testid="desktop-update-status"
					className="text-muted-foreground"
				>
					{note}
				</p>
			)}
			<div className="flex items-center justify-between gap-2">
				<label
					htmlFor={switchId}
					className="cursor-pointer text-muted-foreground"
				>
					{t("ui.desktop_update.auto_label")}
				</label>
				<Switch
					id={switchId}
					checked={current.auto_update}
					data-testid="desktop-update-auto"
					aria-label={t("ui.desktop_update.auto_label")}
					onCheckedChange={(checked) => void onToggleAuto(checked)}
				/>
			</div>
		</section>
	);
}
