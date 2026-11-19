import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { APIRequestContext } from "@playwright/test";

import { caller, ffmpegExe, seedProject } from "./helpers";

// The support of the image editor specs (epic 28): the images and the photos are made through the HTTP API, the
// way an agent or the CLI makes them, so a spec checks what the screen does with a real document.

type Call = Awaited<ReturnType<typeof caller>>;

// testPhoto makes, once per run, a 640x360 still picture, so an image layer has a real file to show.
export function testPhoto(variant = 0): string {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, `photo-${variant}.png`);
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				`gradients=size=640x360:rate=1:seed=${variant + 3},hue=h=${variant * 50}`,
				"-frames:v",
				"1",
				out,
			],
			{ stdio: "ignore" },
		);
	}
	return out;
}

export interface Photo {
	hash: string;
	name: string;
}

// seedPhoto imports the picture into a throwaway project, which puts its file in the library of the workspace
// (an image takes its files from there, SDD-021 section 10), and returns its content hash.
export async function seedPhoto(
	request: APIRequestContext,
	variant = 0,
): Promise<Photo> {
	await seedProject(request, {
		files: [testPhoto(variant)],
		tracks: 0,
		perTrack: 0,
	});
	return photoOf(request, variant);
}

async function photoOf(
	request: APIRequestContext,
	variant: number,
): Promise<Photo> {
	const call = await caller(request);
	const name = `photo-${variant}.png`;
	const library = (await call("GET", "/assets?kind=image")) as {
		assets: { hash: string; name: string }[];
	};
	const found = library.assets.find((a) => a.name.endsWith(name));
	if (!found) throw new Error(`${name} is not in the library`);
	return { hash: found.hash, name: found.name };
}

export interface Made {
	id: string;
	call: Call;
}

// newImage creates a blank document through the API.
export async function newImage(
	request: APIRequestContext,
	body: { name?: string; preset?: string; background?: string } = {},
): Promise<Made> {
	const call = await caller(request, "agent");
	const made = (await call("POST", "/images", {
		name: body.name ?? `E2E image ${Date.now()}`,
		preset: body.preset ?? "video_thumbnail",
		...(body.background === undefined ? {} : { background: body.background }),
	})) as { image_id: string };
	return { id: made.image_id, call };
}

export interface ImageLayer {
	id: string;
	kind: string;
	name: string;
	x: number;
	y: number;
	w: number;
	h: number;
	rotation?: number;
	opacity: number;
	visible: boolean;
	locked?: boolean;
	text?: string;
	fill?: string;
	slot?: string;
	children?: ImageLayer[];
	style?: Record<string, unknown>;
	[key: string]: unknown;
}

export interface ImageState {
	revision: number;
	width: number;
	height: number;
	layers: ImageLayer[];
	findings: { code: string; item?: string }[];
}

export async function imageState(call: Call, id: string): Promise<ImageState> {
	return (await call("GET", `/images/${id}`)) as unknown as ImageState;
}

// editImage sends a batch against the current revision, as an agent does, and returns the new state.
export async function editImage(
	call: Call,
	id: string,
	operations: { op: string; args?: Record<string, unknown> }[],
): Promise<ImageState> {
	const now = await imageState(call, id);
	await call("POST", `/images/${id}/edit`, {
		base_revision: now.revision,
		operations,
	});
	return imageState(call, id);
}

// layerByName finds a layer of the state, at any depth.
export function layerByName(
	layers: ImageLayer[],
	name: string,
): ImageLayer | undefined {
	for (const layer of layers) {
		if (layer.name === name) return layer;
		const inner = layer.children
			? layerByName(layer.children, name)
			: undefined;
		if (inner) return inner;
	}
	return undefined;
}
