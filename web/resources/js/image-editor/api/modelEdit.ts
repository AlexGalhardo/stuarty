import { createClient } from "../../editor/api/client";
import { fromResponse, networkError } from "../../editor/api/errors";
import { HttpTransport } from "../../editor/api/http";
import { TransportError } from "../../editor/api/transport";
import type { Job, ModelView, ProviderView } from "../../editor/api/types";
import type { ProjectSummary } from "../../types";
import { API_BASE } from "./imageApi";

// The operations of an image with a model (SDD-016 section 6.5, epic 30), read and started through the
// client of the video editor, which already has the providers, the projects and the jobs. The only place of
// the image editor that reaches the network for them (rule R6).

export type ModelOperation = "remove_background" | "generate";

export interface ModelEditRequest {
	project_id: string;
	connection_id: string;
	operation: ModelOperation;
	model: string;
	layer_id?: string;
	prompt?: string;
	dry_run?: boolean;
}

export interface ModelEditChoices {
	projects: ProjectSummary[];
	providers: ProviderView[];
}

const START_TIMEOUT_MS = 30_000;

const client = () => createClient(new HttpTransport(API_BASE));

// What the dialog needs when it opens: the active projects, and the providers with their connections.
export async function loadModelEditChoices(): Promise<ModelEditChoices> {
	const [list, providers] = await Promise.all([
		client().listProjects(),
		client().listProviders(),
	]);
	return {
		projects: list.projects.filter((p) => p.state === "active"),
		providers: providers.providers,
	};
}

export async function listModels(connectionId: string): Promise<ModelView[]> {
	return (await client().listProviderModels(connectionId)).models;
}

// Starts the job (or, with dry_run, asks for the one it would make, with its estimate).
export async function startModelEdit(
	imageId: string,
	request: ModelEditRequest,
): Promise<Job> {
	let response: Awaited<ReturnType<HttpTransport["send"]>>;
	try {
		response = await new HttpTransport(API_BASE).send({
			method: "POST",
			path: `/images/${encodeURIComponent(imageId)}/model-edit`,
			body: request,
			timeoutMs: START_TIMEOUT_MS,
		});
	} catch (error) {
		if (error instanceof TransportError) throw networkError(error.message);
		throw error;
	}
	if (response.status < 200 || response.status > 299) {
		throw fromResponse(response.status, response.body);
	}
	return response.body as Job;
}

export function getModelJob(jobId: string): Promise<Job> {
	return client().getJob(jobId);
}
