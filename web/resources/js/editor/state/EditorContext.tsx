import { createContext, type ReactNode, useContext } from "react";
import type { ActivityState } from "./activityStore";
import { useStore } from "./createStore";
import type { DocumentState } from "./documentStore";
import type { EditorRuntime } from "./runtime";
import type { UiState } from "./uiStore";

const EditorContext = createContext<EditorRuntime | null>(null);

export function EditorProvider({
	runtime,
	children,
}: {
	runtime: EditorRuntime;
	children?: ReactNode;
}): ReactNode {
	return (
		<EditorContext.Provider value={runtime}>{children}</EditorContext.Provider>
	);
}

export function useRuntime(): EditorRuntime {
	const runtime = useContext(EditorContext);
	if (runtime === null) throw new Error("EditorProvider is missing");
	return runtime;
}

export function useDocumentState<T>(select: (state: DocumentState) => T): T {
	return useStore(useRuntime().documents, select);
}

export function useUiState<T>(select: (state: UiState) => T): T {
	return useStore(useRuntime().ui, select);
}

export function useActivityState<T>(select: (state: ActivityState) => T): T {
	return useStore(useRuntime().activity, select);
}
