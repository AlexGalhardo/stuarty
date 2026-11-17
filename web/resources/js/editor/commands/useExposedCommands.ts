import { useEffect, useState } from "react";

import type { ApiClient } from "../api/client";
import { useRuntime } from "../state/EditorContext";

// The names the service lists, read once per client. A control of a command
// that a build does not expose (the BETA item commands in an ALPHA build) is
// drawn only when its name is here, so the UI follows the live registry and
// never a build constant (SDD-012 section 9.2).
const cache = new WeakMap<ApiClient, Promise<ReadonlySet<string>>>();

function load(client: ApiClient): Promise<ReadonlySet<string>> {
	let names = cache.get(client);
	if (names === undefined) {
		names = client.getCommands().then(
			(result) => new Set((result.commands ?? []).map((c) => c.name)),
			() => {
				cache.delete(client);
				return new Set<string>();
			},
		);
		cache.set(client, names);
	}
	return names;
}

// The names the registry lists; empty until the list arrives.
export function useCommandNames(): ReadonlySet<string> {
	const runtime = useRuntime();
	const [exposed, setExposed] = useState<ReadonlySet<string>>(new Set());
	useEffect(() => {
		let current = true;
		void load(runtime.client).then((set) => {
			if (current) setExposed(set);
		});
		return () => {
			current = false;
		};
	}, [runtime]);
	return exposed;
}

// Whether the registry the service sent holds every one of `names`.
export function useCommandsExposed(...names: string[]): boolean {
	const exposed = useCommandNames();
	return names.every((name) => exposed.has(name));
}

const details = new WeakMap<ApiClient, Map<string, Promise<unknown>>>();

function loadDetail(client: ApiClient, command: string): Promise<unknown> {
	let byName = details.get(client);
	if (byName === undefined) {
		byName = new Map();
		details.set(client, byName);
	}
	let schema = byName.get(command);
	if (schema === undefined) {
		schema = client.getCommand(command).then(
			(result) => result.command?.args_schema ?? null,
			() => {
				byName.delete(command);
				return null;
			},
		);
		byName.set(command, schema);
	}
	return schema;
}

// The string values of one argument of a command, read from the registry the
// service sent (`args_schema.properties[arg].enum`): the kinds, filters and
// presets a panel lists are the core's, never a copy (SDD-012 section 6.4).
// Empty until the registry answers, and for an argument with no enum.
export function useCommandEnum(
	command: string,
	...path: string[]
): readonly string[] {
	const runtime = useRuntime();
	const [values, setValues] = useState<readonly string[]>([]);
	const key = path.join(".");
	// biome-ignore lint/correctness/useExhaustiveDependencies: `key` stands for `path`, which is a new array on every call.
	useEffect(() => {
		let current = true;
		void loadDetail(runtime.client, command).then((schema) => {
			if (!current) return;
			let node = schema as EnumSchema | null | undefined;
			for (const step of path) {
				node = step === "items" ? node?.items : node?.properties?.[step];
			}
			setValues(
				(node?.enum ?? []).filter(
					(value): value is string => typeof value === "string",
				),
			);
		});
		return () => {
			current = false;
		};
	}, [runtime, command, key]);
	return values;
}

interface EnumSchema {
	enum?: unknown[];
	properties?: Record<string, EnumSchema>;
	items?: EnumSchema;
}
