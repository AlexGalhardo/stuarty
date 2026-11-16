import type { Track } from "@/generated/timeline";
import type { ApiError } from "../../api/errors";
import type { MediaInspectResult } from "../../api/types";

// The track a new caption batch targets when the person has not chosen one.
export const NEW_TRACK = "";

// SDD-012 section 12.4: the first visual track that has a `caption_style`, else
// a new track (the empty id).
export function defaultCaptionTrack(
	orderedIds: readonly string[],
	tracks: Readonly<Record<string, Track>>,
): string {
	for (const id of orderedIds) {
		const track = tracks[id];
		if (track?.kind === "visual" && track.caption_style !== undefined)
			return id;
	}
	return NEW_TRACK;
}

// An asset already has a transcript when the inspection lists the section.
export function hasTranscript(inspect: MediaInspectResult): boolean {
	return inspect.available.includes("transcript");
}

// What the install dialog shows, from the details of E_TRANSCRIBE_UNAVAILABLE
// (SDD-012 section 12.5). Every part is optional: a platform without a helper
// answers the code with a hint and nothing to install.
export interface Unavailable {
	message: string;
	hint?: string;
	missing: { name: string; host?: string; bytes?: number }[];
	downloadBytes?: number;
	installCommand?: string;
	// The tool and model the service names for the install (`details.tool`,
	// `details.model`, SDD-012 §12.5).
	tool?: string;
	model?: string;
	code: string;
	details?: unknown;
}

export function unavailableFrom(error: {
	code: string;
	message: string;
	hint?: string;
	details?: ApiError["details"];
}): Unavailable {
	const details = error.details ?? {};
	const rawMissing = Array.isArray(details.missing) ? details.missing : [];
	const missing = rawMissing.flatMap((entry) => {
		if (typeof entry !== "object" || entry === null || Array.isArray(entry)) {
			return [];
		}
		const { name, url_host: host, bytes } = entry;
		if (typeof name !== "string") return [];
		return [
			{
				name,
				host: typeof host === "string" ? host : undefined,
				bytes: typeof bytes === "number" ? bytes : undefined,
			},
		];
	});
	return {
		code: error.code,
		message: error.message,
		hint: error.hint,
		missing,
		downloadBytes:
			typeof details.download_bytes === "number"
				? details.download_bytes
				: undefined,
		installCommand:
			typeof details.install_command === "string"
				? details.install_command
				: undefined,
		tool: typeof details.tool === "string" ? details.tool : undefined,
		model: typeof details.model === "string" ? details.model : undefined,
		details: error.details,
	};
}

// The whisper models `tools:install --model` accepts. The service does not list
// them over HTTP, so this is the list the CLI help gives (engine/transcribe).
export const WHISPER_MODELS: readonly string[] = [
	"tiny",
	"base",
	"small",
	"large-v3-turbo",
];
export const DEFAULT_WHISPER_MODEL = "small";

// The install request: the tool and model the service names, or, from an older
// server that does not send them, the ones read from its command.
export function installRequest(
	unavailable: Unavailable,
): { name: string; model?: string } | null {
	if (unavailable.tool !== undefined && unavailable.tool !== "") {
		return unavailable.model === undefined || unavailable.model === ""
			? { name: unavailable.tool }
			: { name: unavailable.tool, model: unavailable.model };
	}
	return parseInstallCommand(unavailable.installCommand);
}

const INSTALL_COMMAND = /^vukem-video tools:install (\S+)(?: --model (\S+))?$/;

// The tool and the model the install step takes, read from the command the
// service announced (`details.install_command`); null when the command is not
// the one the service writes, so no request is built from guesswork.
export function parseInstallCommand(
	command: string | undefined,
): { name: string; model?: string } | null {
	const match = command === undefined ? null : INSTALL_COMMAND.exec(command);
	if (match === null) return null;
	const [, name, model] = match;
	return model === undefined
		? { name: name as string }
		: { name: name as string, model };
}
