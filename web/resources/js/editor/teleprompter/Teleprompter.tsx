import { Pause, Play, RotateCcw } from "lucide-react";
import {
	type ReactNode,
	type RefObject,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { LoadingState } from "@/components/LoadingState";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import type { WritingClient } from "../../pages/Write/client";
import {
	browserStorage,
	loadPrefs,
	SIZE_MAX,
	SIZE_MIN,
	SPEED_MAX,
	SPEED_MIN,
	savePrefs,
	type TeleprompterPrefs,
} from "./prefs";
import { loadScript, type ProjectScript } from "./script";
import { advance } from "./scroll";

type Loaded =
	| { state: "loading" }
	| { state: "none" }
	| { state: "ready"; script: ProjectScript }
	| { state: "failed"; failure: Failure };

// The teleprompter (SDD-025 section 20.3): the script of the project, scrolled at the speed the person set, with the
// size and the mirror they chose. It reads the script with the writing operations that exist and writes nothing; the
// text is drawn as text, never as markup. The scroll is an animation of the page and decides nothing about an edit.
export default function Teleprompter({
	client,
	projectId,
	className = "",
}: {
	client: Pick<WritingClient, "list" | "get">;
	projectId: string;
	className?: string;
}): ReactNode {
	const t = useT();
	const [loaded, setLoaded] = useState<Loaded>({ state: "loading" });
	const [prefs, setPrefs] = useState<TeleprompterPrefs>(() =>
		loadPrefs(browserStorage()),
	);
	const [playing, setPlaying] = useState(false);
	const view = useRef<HTMLElement>(null);
	const text = useRef<HTMLDivElement>(null);
	const offset = useRef(0);
	const speed = useRef(prefs.speed);
	speed.current = prefs.speed;

	useEffect(() => {
		let current = true;
		setLoaded({ state: "loading" });
		loadScript(client, projectId).then(
			(script) => {
				if (current)
					setLoaded(
						script === null ? { state: "none" } : { state: "ready", script },
					);
			},
			(error: unknown) => {
				if (current) setLoaded({ state: "failed", failure: failureOf(error) });
			},
		);
		return () => {
			current = false;
		};
	}, [client, projectId]);

	// place scrolls the text to an offset, held between the start and the end, and returns the end. The box scrolls
	// by itself too (the wheel, the keyboard), so the offset is read from it when the scroll starts.
	const place = useCallback((next: number): number => {
		const box = view.current;
		if (!box) return 0;
		const limit = Math.max(0, box.scrollHeight - box.clientHeight);
		offset.current = Math.min(limit, Math.max(0, next));
		box.scrollTop = offset.current;
		box.dataset.offset = String(Math.round(offset.current * 100) / 100);
		return limit;
	}, []);

	useEffect(() => {
		if (!playing) return;
		offset.current = view.current?.scrollTop ?? 0;
		let last = performance.now();
		let frame = 0;
		const tick = (now: number): void => {
			const box = view.current;
			const limit = Math.max(
				0,
				(box?.scrollHeight ?? 0) - (box?.clientHeight ?? 0),
			);
			const step = advance(offset.current, now - last, speed.current, limit);
			last = now;
			place(step.offset);
			if (step.done) {
				setPlaying(false);
				return;
			}
			frame = requestAnimationFrame(tick);
		};
		frame = requestAnimationFrame(tick);
		return () => cancelAnimationFrame(frame);
	}, [playing, place]);

	const change = (patch: Partial<TeleprompterPrefs>): void => {
		const next = { ...prefs, ...patch };
		setPrefs(next);
		savePrefs(browserStorage(), next);
	};

	if (loaded.state === "loading") {
		return (
			<LoadingState
				message={t("ui.editor.teleprompter.loading")}
				className={className}
			/>
		);
	}
	if (loaded.state === "failed") {
		if (loaded.failure.code === "E_NOT_FOUND") {
			return <ProjectNotFound className={className} />;
		}
		return <FailureNote failure={loaded.failure} />;
	}
	if (loaded.state === "none") return <NoScript className={className} />;

	return (
		<TeleprompterView
			script={loaded.script}
			prefs={prefs}
			playing={playing}
			className={className}
			viewRef={view}
			textRef={text}
			onToggle={() => setPlaying((on) => !on)}
			onRestart={() => {
				setPlaying(false);
				place(0);
			}}
			onChange={change}
		/>
	);
}

// What a project that does not exist shows (VV-441): words and a way back, never the route of the API.
export function ProjectNotFound({
	className = "",
}: {
	className?: string;
}): ReactNode {
	const t = useT();
	return (
		<div
			className={`${className} flex flex-col items-start gap-3`}
			data-slot="teleprompter-not-found"
		>
			<h1 className="text-lg font-semibold">
				{t("ui.editor.teleprompter.not_found_title")}
			</h1>
			<p className="text-sm text-text-muted">
				{t("ui.editor.teleprompter.not_found_body")}
			</p>
			<Button asChild>
				<a href="/projects">{t("ui.editor.teleprompter.not_found_back")}</a>
			</Button>
		</div>
	);
}

// What a project with no script shows.
export function NoScript({
	className = "",
}: {
	className?: string;
}): ReactNode {
	const t = useT();
	return (
		<div className={className} data-slot="teleprompter-empty">
			<p className="text-sm font-medium">
				{t("ui.editor.teleprompter.no_script")}
			</p>
			<p className="text-sm text-text-muted">
				{t("ui.editor.teleprompter.no_script_hint")}
			</p>
		</div>
	);
}

// The controls and the text. It draws what it is given and runs nothing, so the tests draw it with any state.
export function TeleprompterView({
	script,
	prefs,
	playing,
	className = "",
	viewRef,
	textRef,
	onToggle,
	onRestart,
	onChange,
}: {
	script: ProjectScript;
	prefs: TeleprompterPrefs;
	playing: boolean;
	className?: string;
	viewRef?: RefObject<HTMLElement | null>;
	textRef?: RefObject<HTMLDivElement | null>;
	onToggle: () => void;
	onRestart: () => void;
	onChange: (patch: Partial<TeleprompterPrefs>) => void;
}): ReactNode {
	const t = useT();
	return (
		<section
			aria-label={t("ui.editor.teleprompter.title")}
			data-slot="teleprompter"
			className={`flex min-h-0 flex-col gap-3 ${className}`}
		>
			<div className="flex flex-wrap items-end gap-x-4 gap-y-2 text-sm">
				<Button
					type="button"
					variant="outline"
					size="sm"
					aria-pressed={playing}
					onClick={onToggle}
				>
					{playing ? <Pause aria-hidden="true" /> : <Play aria-hidden="true" />}
					{playing
						? t("ui.editor.teleprompter.pause")
						: t("ui.editor.teleprompter.play")}
				</Button>
				<Button type="button" variant="ghost" size="sm" onClick={onRestart}>
					<RotateCcw aria-hidden="true" />
					{t("ui.editor.teleprompter.restart")}
				</Button>
				<label className="flex flex-col gap-1">
					<span>
						{t("ui.editor.teleprompter.speed")}: {prefs.speed}
					</span>
					<input
						type="range"
						min={SPEED_MIN}
						max={SPEED_MAX}
						step={10}
						value={prefs.speed}
						onChange={(event) =>
							onChange({ speed: Number(event.target.value) })
						}
					/>
				</label>
				<label className="flex flex-col gap-1">
					<span>
						{t("ui.editor.teleprompter.size")}: {prefs.size}
					</span>
					<input
						type="range"
						min={SIZE_MIN}
						max={SIZE_MAX}
						step={2}
						value={prefs.size}
						onChange={(event) => onChange({ size: Number(event.target.value) })}
					/>
				</label>
				<label className="flex items-center gap-2">
					<input
						type="checkbox"
						checked={prefs.mirror}
						onChange={(event) => onChange({ mirror: event.target.checked })}
					/>
					{t("ui.editor.teleprompter.mirror")}
				</label>
			</div>
			<section
				ref={viewRef}
				aria-label={script.title}
				data-mirror={prefs.mirror ? "true" : "false"}
				className="min-h-48 flex-1 overflow-y-auto rounded-lg border bg-black text-white outline-offset-2"
			>
				<div
					ref={textRef}
					data-slot="teleprompter-text"
					style={{
						fontSize: `${prefs.size}px`,
						lineHeight: 1.35,
						...(prefs.mirror ? { scale: "-1 1" } : {}),
					}}
					className="whitespace-pre-wrap p-4"
				>
					<UntrustedText>{script.body}</UntrustedText>
				</div>
			</section>
		</section>
	);
}
