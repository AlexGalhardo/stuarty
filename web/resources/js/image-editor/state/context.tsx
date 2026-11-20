import { createContext, type ReactNode, useContext } from "react";
import { useStore } from "zustand";

import type { ImageApi } from "../api/imageApi";
import type { Layer } from "../api/types";
import type { ImageActions, ImageState, ImageStore } from "./store";

// What the components of the image editor share: the store of the document and the client of the API.
interface Value {
	store: ImageStore;
	api: ImageApi;
}

const Context = createContext<Value | null>(null);

export function ImageEditorProvider({
	store,
	api,
	children,
}: {
	store: ImageStore;
	api: ImageApi;
	children: ReactNode;
}): ReactNode {
	return <Context.Provider value={{ store, api }}>{children}</Context.Provider>;
}

function useValue(): Value {
	const v = useContext(Context);
	if (v === null) throw new Error("ImageEditorProvider is missing");
	return v;
}

export function useImage<T>(
	selector: (state: ImageState & ImageActions) => T,
): T {
	return useStore(useValue().store, selector);
}

export function useImageStore(): ImageStore {
	return useValue().store;
}

export function useImageApi(): ImageApi {
	return useValue().api;
}

// A layer of the read model by id, searching the groups.
export function findLayer(
	layers: readonly Layer[],
	id: string,
): Layer | undefined {
	for (const layer of layers) {
		if (layer.id === id) return layer;
		const found = layer.children ? findLayer(layer.children, id) : undefined;
		if (found) return found;
	}
	return undefined;
}
