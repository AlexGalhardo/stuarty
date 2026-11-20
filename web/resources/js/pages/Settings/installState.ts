import type { InstalledTool } from "../../editor/api/types";

// What the workspace holds for local transcription, read from the installed list the service
// answers (`GET /system/tools`): the helper program and the models downloaded for it. The page
// only reports it; the service decides what is installed.
export interface TranscriptionState {
	helper: boolean;
	models: string[];
}

export function transcriptionState(
	installed: readonly InstalledTool[],
): TranscriptionState {
	return {
		helper: installed.some((tool) => tool.kind === "helper"),
		models: installed
			.filter((tool) => tool.kind === "model" && tool.model !== undefined)
			.map((tool) => tool.model as string)
			.sort(),
	};
}

// The command a person runs in a terminal when this session cannot install: the server refuses a
// local token and every agent surface (SDD-007 section 6.8).
export function installCommand(model: string): string {
	return `stuarty tools:install whisper --model ${model}`;
}
