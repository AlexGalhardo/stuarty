import {
	Ban,
	Bot,
	CircleAlert,
	CircleCheck,
	Hourglass,
	LoaderCircle,
	type LucideIcon,
	MessageCircle,
	Sparkles,
	SquareTerminal,
} from "lucide-react";
import { type ReactNode, useId } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { PersonOnlyNote, useSession } from "@/lib/session";
import type { SceneModel, SceneVariant } from "./sceneState";

const LOOK: Record<
	SceneVariant,
	{ icon: LucideIcon; tone: string; text: string }
> = {
	none: { icon: Bot, tone: "bg-surface-sunken", text: "text-text-muted" },
	idle: {
		icon: MessageCircle,
		tone: "bg-surface-sunken",
		text: "text-agent-text",
	},
	working: {
		icon: LoaderCircle,
		tone: "bg-surface-sunken",
		text: "text-agent-text",
	},
	waiting: {
		icon: Hourglass,
		tone: "bg-warning-soft",
		text: "text-warning-text",
	},
	denied: { icon: Ban, tone: "bg-surface-sunken", text: "text-danger-text" },
	done: {
		icon: CircleCheck,
		tone: "bg-surface-sunken",
		text: "text-success-text",
	},
	failed: {
		icon: CircleAlert,
		tone: "bg-danger-soft",
		text: "text-danger-text",
	},
};

function Slot({
	icon: Icon,
	label,
	testId,
	children,
	className,
}: {
	icon: LucideIcon;
	label: string;
	testId: string;
	children: ReactNode;
	className?: string;
}): ReactNode {
	return (
		<div
			data-testid={testId}
			className={cn("flex min-w-0 items-center gap-2", className)}
		>
			<Icon aria-hidden="true" className="size-4 shrink-0 text-icon" />
			<div className="min-w-0">
				<p className="text-xs text-muted-foreground">{label}</p>
				<p className="truncate text-sm font-medium">{children}</p>
			</div>
		</div>
	);
}

// The scene bar (SDD-012 section 23.4): the agent, the state of its session, the skill, the tool and, for an
// open permission request, Allow and Deny. It reports what the service says and nothing more: the skill is
// "Not reported" until the server reports one, and there is no next step, no cost and no "always allow".
export default function SceneBar({
	model,
	connected,
	busy,
	onAnswer,
	onOpenChat,
}: {
	model: SceneModel;
	// whether an agent profile exists: null until the list was read
	connected: boolean | null;
	busy: boolean;
	onAnswer(requestId: string, answer: "allow" | "reject"): void;
	onOpenChat(): void;
}): ReactNode {
	const t = useT();
	const { privileged } = useSession();
	const ids = useId();
	const { variant, request } = model;
	const look = LOOK[variant];
	const State = look.icon;
	const personOnly = privileged ? undefined : `${ids}-person-only`;
	const asking = variant === "waiting" && request !== null;

	const reason =
		variant === "failed" && model.endReason !== null
			? ` · ${t(`ui.chat.ended_${model.endReason}`)}`
			: "";
	// Said to a reader only when a person is needed: a running agent changes tool all the time.
	const announcement =
		variant === "waiting" || variant === "failed"
			? `${model.agent ?? ""}: ${t(`ui.shell.scene.state.${variant}`)}${reason}${
					request?.title ? `: ${request.title}` : ""
				}`
			: "";

	return (
		<section
			aria-label={t("ui.shell.scene.label")}
			data-testid="scene-bar"
			data-variant={variant}
			className={cn(
				"sticky bottom-0 z-20 flex shrink-0 flex-wrap items-center gap-x-6 gap-y-2 border-t px-4 py-2 md:h-14 md:flex-nowrap md:py-0",
				look.tone,
			)}
		>
			<div
				data-testid="scene-agent"
				className="flex min-w-0 items-center gap-2"
			>
				<State
					aria-hidden="true"
					className={cn(
						"size-5 shrink-0",
						look.text,
						variant === "working" && "motion-safe:animate-spin",
					)}
				/>
				<div className="min-w-0">
					<p className="truncate text-sm font-medium">
						{model.agent === null ? (
							t("ui.shell.scene.none")
						) : (
							<UntrustedText>{model.agent}</UntrustedText>
						)}
					</p>
					<p
						data-testid="scene-state"
						className={cn("truncate text-xs font-medium", look.text)}
					>
						{variant === "none"
							? t("ui.shell.scene.none_hint")
							: `${t(`ui.shell.scene.state.${variant}`)}${reason}`}
					</p>
				</div>
			</div>
			{variant !== "none" && (
				<>
					<Slot
						icon={Sparkles}
						label={t("ui.shell.scene.skill")}
						testId="scene-skill"
						className="max-md:hidden"
					>
						<span className="text-muted-foreground">
							{t("ui.shell.scene.not_reported")}
						</span>
					</Slot>
					<Slot
						icon={SquareTerminal}
						label={
							model.tool === null
								? t("ui.shell.scene.tool")
								: `${t("ui.shell.scene.tool")} · ${t(`ui.chat.tool_status_${model.tool.status}`)}`
						}
						testId="scene-tool"
						className="min-w-0 flex-1"
					>
						{model.tool === null ? (
							<span className="text-muted-foreground">
								{t("ui.shell.scene.no_tool")}
							</span>
						) : (
							<UntrustedText>{model.tool.title}</UntrustedText>
						)}
					</Slot>
				</>
			)}
			<div className="ms-auto flex flex-wrap items-center gap-2 max-md:w-full max-md:justify-end">
				{variant === "none" && (
					<Button
						type="button"
						size="sm"
						variant="outline"
						onClick={onOpenChat}
						data-testid="scene-open-chat"
					>
						{connected === false
							? t("ui.shell.scene.connect")
							: t("ui.shell.scene.open_chat")}
					</Button>
				)}
				{asking && (
					<>
						<Button
							type="button"
							size="sm"
							variant="outline"
							disabled={busy || personOnly !== undefined}
							aria-describedby={personOnly}
							onClick={() => onAnswer(request.requestId, "reject")}
							data-testid="scene-deny"
						>
							{t("ui.shell.scene.deny")}
						</Button>
						<Button
							type="button"
							size="sm"
							disabled={busy || personOnly !== undefined}
							aria-describedby={personOnly}
							onClick={() => onAnswer(request.requestId, "allow")}
							data-testid="scene-allow"
						>
							{t("ui.shell.scene.allow")}
						</Button>
					</>
				)}
				{variant !== "none" && !asking && (
					<Button
						type="button"
						size="sm"
						variant="ghost"
						onClick={onOpenChat}
						data-testid="scene-open-chat"
					>
						{t("ui.shell.scene.open_chat")}
					</Button>
				)}
			</div>
			{asking && (
				<div className="basis-full">
					<PersonOnlyNote id={`${ids}-person-only`} />
				</div>
			)}
			<p role="status" className="sr-only" data-testid="scene-announce">
				{announcement}
			</p>
		</section>
	);
}
