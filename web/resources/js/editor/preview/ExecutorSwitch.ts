import type { ApiClient } from "../api/client";
import { readPreviewExecutor } from "../state/prefs";
import {
	chooseExecutor,
	type ExecutorChoice,
	missingFeatures,
	probeFeatures,
	readSetting,
} from "./executorChoice";
import { PixiExecutor } from "./PixiExecutor";
import { RustExecutor } from "./RustExecutor";
import type {
	LayerGeometry,
	PreviewExecutor,
	PreviewState,
	ProjectInfo,
	Unsubscribe,
} from "./types";

type Sources = (asset: string) => { width: number; height: number } | null;

// The executor of the editor: the one `chooseExecutor` picks, and PixiJS in its
// place when the renderer's worker cannot start (a module that does not load, a
// worker the page may not create). It keeps what the preview was told so the
// second executor starts from the same state, and tells which one draws on the
// canvas (`data-executor`, `data-executor-reason`).
export class ExecutorSwitch implements PreviewExecutor {
	private current: PreviewExecutor;
	private unsubscribe: Unsubscribe[] = [];
	private readonly frameListeners = new Set<(frame: number) => void>();
	private readonly stateListeners = new Set<(state: PreviewState) => void>();
	private project: ProjectInfo | null = null;
	private revision: number | null = null;
	private buffer: { width: number; height: number } | null = null;
	private volume: { volume: number; muted: boolean } | null = null;
	private lastFrame = 0;
	private reason: string | null;
	private disposed = false;

	constructor(
		choice: ExecutorChoice,
		private readonly make: (kind: "rust" | "pixi") => PreviewExecutor,
	) {
		this.reason = choice.reason;
		if (choice.reason !== null) console.info("preview:", choice.reason);
		this.current = make(choice.kind);
		this.kind = choice.kind;
		this.attach();
	}

	private kind: "rust" | "pixi";

	private attach(): void {
		for (const off of this.unsubscribe) off();
		this.unsubscribe = [
			this.current.onFrame((frame) => {
				this.lastFrame = frame;
				for (const cb of this.frameListeners) cb(frame);
			}),
			this.current.onState((state) => {
				for (const cb of this.stateListeners) cb(state);
			}),
		];
	}

	get stats(): PreviewExecutor["stats"] {
		return this.current.stats;
	}

	async mount(host: HTMLElement): Promise<void> {
		try {
			await this.current.mount(host);
		} catch (err) {
			if (this.kind === "pixi" || this.disposed) throw err;
			this.reason = `the renderer worker did not start: ${
				err instanceof Error ? err.message : String(err)
			}`;
			console.warn("preview:", this.reason);
			this.current.dispose();
			this.kind = "pixi";
			this.current = this.make("pixi");
			this.attach();
			if (this.project !== null) this.current.setProject(this.project);
			if (this.revision !== null) this.current.setRevision(this.revision);
			if (this.buffer !== null) {
				this.current.resize(this.buffer.width, this.buffer.height);
			}
			if (this.volume !== null) {
				this.current.setVolume(this.volume.volume, this.volume.muted);
			}
			await this.current.mount(host);
			this.current.seek(this.lastFrame);
		}
		const canvas = host.querySelector("canvas");
		canvas?.setAttribute("data-executor", this.kind);
		if (this.reason !== null) {
			canvas?.setAttribute("data-executor-reason", this.reason);
		}
	}

	dispose(): void {
		this.disposed = true;
		for (const off of this.unsubscribe) off();
		this.unsubscribe = [];
		this.frameListeners.clear();
		this.stateListeners.clear();
		this.current.dispose();
	}

	setProject(info: ProjectInfo): void {
		this.project = info;
		this.current.setProject(info);
	}

	setRevision(revision: number): void {
		this.revision = revision;
		this.current.setRevision(revision);
	}

	refresh(): void {
		this.current.refresh();
	}

	seek(frame: number, options?: { scrubbing?: boolean }): void {
		this.lastFrame = frame;
		this.current.seek(frame, options);
	}

	play(): Promise<void> {
		return this.current.play();
	}

	pause(): void {
		this.current.pause();
	}

	onFrame(cb: (frame: number) => void): Unsubscribe {
		this.frameListeners.add(cb);
		return () => this.frameListeners.delete(cb);
	}

	onState(cb: (state: PreviewState) => void): Unsubscribe {
		this.stateListeners.add(cb);
		return () => this.stateListeners.delete(cb);
	}

	setVolume(volume: number, muted: boolean): void {
		this.volume = { volume, muted };
		this.current.setVolume(volume, muted);
	}

	resize(bufferWidth: number, bufferHeight: number): void {
		this.buffer = { width: bufferWidth, height: bufferHeight };
		this.current.resize(bufferWidth, bufferHeight);
	}

	getLayerGeometry(frame: number): LayerGeometry[] {
		return this.current.getLayerGeometry(frame);
	}

	getSafeArea(frame: number): [number, number, number, number] | null {
		return this.current.getSafeArea(frame);
	}

	whenPlan(frame: number): Promise<void> {
		return this.current.whenPlan(frame);
	}

	readColour(x: number, y: number): string | null {
		return this.current.readColour(x, y);
	}
}

// The executor for this page: the setting (query, then storage) and the probe
// of the browser decide, once.
export function createPreviewExecutor(
	client: ApiClient,
	sources: Sources,
): PreviewExecutor {
	const setting = readSetting(window.location.search, readPreviewExecutor());
	const missing = missingFeatures(
		probeFeatures(window as unknown as Record<string, unknown>),
	);
	return new ExecutorSwitch(chooseExecutor(setting, missing), (kind) =>
		kind === "rust"
			? new RustExecutor(client, undefined, sources)
			: new PixiExecutor(client, undefined, sources),
	);
}
