import {
	ArrowRight,
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
				<p className="truncate text-sm text-muted-foreground">{label}</p>
				<p className="flex min-w-0 items-baseline truncate text-sm font-medium">
					{children}
				</p>
			</div>
		</div>
	);
}

// The line above the tool: its technical name (or that the title is the agent's own text) and its status.
function toolLabel(model: SceneModel, t: (key: string) => string): string {
	const { tool } = model;
	if (tool === null) return t("ui.shell.scene.tool");
	const who = tool.technicalName ?? t("ui.shell.scene.tool_from_agent");
	return `${t("ui.shell.scene.tool")} · ${who} · ${t(`ui.chat.tool_status_${tool.status}`)}`;
}

// The scene bar (SDD-012 section 23.4): the agent, the state of its session, the skill, the tool and, for an
// open permission request, Allow and Deny. It reports what the service says and nothing more: the skill and
// the tool come from the activity of the session and are "Not reported" until the server answers, a skill is
// "loaded in this session" and never "in use", and there is no next step, no cost and no "always allow".
export default function SceneBar({
	model,
	project = null,
	connected,
	busy,
	onAnswer,
	onOpenChat,
	next = null,
}: {
	model: SceneModel;
	// what comes after this step, in words (untrusted text), when the page knows it; the bar never guesses one
	next?: string | null;
	// the name of the project of the session (untrusted text): the page may be about another one, or none
	project?: string | null;
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
	// The reason a session that is not a person's cannot answer takes a line of its own; with none, the bar
	// keeps one row of 56 px (a full-width element in that row would crush the slots and the actions).
	const note = asking && personOnly !== undefined;

	const reason =
		variant === "failed" && model.endReason !== null
			? ` · ${t(`ui.chat.ended_${model.endReason}`)}`
			: "";
	// Said to a reader only when a person is needed: a running agent changes tool all the time.
	const announcement =
		variant === "waiting" || variant === "failed"
			? `${model.agent ?? ""}${project === null ? "" : ` (${project})`}: ${t(`ui.shell.scene.state.${variant}`)}${reason}${
					request?.title ? `: ${request.title}` : ""
				}`
			: "";

	return (
		<section
			aria-label={t("ui.shell.scene.label")}
			data-testid="scene-bar"
			data-variant={variant}
			className={cn(
				// A window under 30 rem high (a phone on its side, 400 % zoom) keeps the bar at the end of the page:
				// stuck to the bottom, its card would hide most of what is left between it and the top bar.
				"sticky bottom-0 z-20 flex shrink-0 flex-wrap items-center gap-x-6 gap-y-2 border-t px-4 py-2 [@media(max-height:30rem)]:static",
				note ? "md:min-h-14" : "md:h-14 md:flex-nowrap md:py-0",
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
					{/* relative: the sr-only word of the project is absolute, and would escape the clip of a static line */}
					<p className="relative flex min-w-0 items-baseline text-sm font-medium">
						{model.agent === null ? (
							<span className="truncate">{t("ui.shell.scene.none")}</span>
						) : (
							<UntrustedText className="min-w-0 shrink truncate">
								{model.agent}
							</UntrustedText>
						)}
						{model.agent !== null && project !== null && (
							<span
								data-testid="scene-project"
								className="flex min-w-0 items-baseline font-normal text-muted-foreground"
							>
								<span className="shrink-0 whitespace-pre">{" · "}</span>
								<span className="sr-only">{t("ui.shell.scene.project")} </span>
								<UntrustedText className="min-w-0 truncate">
									{project}
								</UntrustedText>
							</span>
						)}
					</p>
					<p
						data-testid="scene-state"
						className={cn("truncate text-xs font-medium", look.text)}
					>
						{variant === "none"
							? // with no agent connected, nothing waits for a request: the bar says so instead
								t(
									connected === false
										? "ui.shell.scene.none_unconnected"
										: "ui.shell.scene.none_hint",
								)
							: `${t(`ui.shell.scene.state.${variant}`)}${reason}`}
					</p>
				</div>
			</div>
			<Slot
				icon={Sparkles}
				label={
					model.skill === null
						? t("ui.shell.scene.skill")
						: `${t("ui.shell.scene.skill")} · ${t(`ui.shell.scene.skill_state.${model.skill.state}`)}`
				}
				testId="scene-skill"
				className="max-md:hidden"
			>
				{model.skill === null ? (
					<span className="text-muted-foreground">
						{t(
							variant === "none"
								? "ui.shell.scene.skill_not_in_use"
								: "ui.shell.scene.not_reported",
						)}
					</span>
				) : model.skill.state === "loaded" ? (
					<UntrustedText className="min-w-0 truncate">
						{model.skill.name}
					</UntrustedText>
				) : (
					<span className="text-muted-foreground">
						{t(
							model.skill.state === "available"
								? "ui.shell.scene.skill_available"
								: "ui.shell.scene.skill_none",
						)}
					</span>
				)}
			</Slot>
			<Slot
				icon={SquareTerminal}
				label={toolLabel(model, t)}
				testId="scene-tool"
				// 7 rem at least: squeezed beside a long agent line it goes to a line of its own
				className="min-w-28 flex-1"
			>
				{model.tool === null ? (
					<span className="text-muted-foreground">
						{variant === "none"
							? t("ui.shell.scene.tool_not_in_use")
							: model.reported
								? t("ui.shell.scene.no_tool")
								: t("ui.shell.scene.not_reported")}
					</span>
				) : model.tool.descriptionKey !== null ? (
					// the plain description of a tool of Stuarty, in words a person can judge, and the title the agent
					// gave the call: the tool is recognised from that title, so an agent can claim the name of a tool
					// of Stuarty, and the person sees what it wrote
					<>
						<span className="shrink-0">{t(model.tool.descriptionKey)}</span>
						<span className="flex min-w-0 items-baseline font-normal text-muted-foreground">
							<span className="shrink-0 whitespace-pre">{" · "}</span>
							<UntrustedText className="min-w-0 truncate">
								{model.tool.title}
							</UntrustedText>
						</span>
					</>
				) : (
					<UntrustedText className="min-w-0 truncate">
						{model.tool.title}
					</UntrustedText>
				)}
			</Slot>
			<Slot
				icon={ArrowRight}
				label={t("ui.shell.scene.next")}
				testId="scene-next"
				className="max-lg:hidden"
			>
				{next === null ? (
					<span className="text-muted-foreground">
						{t("ui.shell.scene.next_none")}
					</span>
				) : (
					<UntrustedText className="min-w-0 truncate">{next}</UntrustedText>
				)}
			</Slot>
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
			{note && (
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
