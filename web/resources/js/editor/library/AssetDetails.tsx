import { type ReactNode, useEffect, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { useT } from "@/lib/i18n";
import { ApiError } from "../api/errors";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { errorHeadline } from "../ui/errorHeadline";

// "Inspect" of a card: the text the core makes of `inspectMedia` (SDD-012
// section 13.2). The text is the core's and stays in English.
export default function AssetDetails({
	assetId,
	name,
	open,
	onOpenChange,
}: {
	assetId: string;
	name: string;
	open: boolean;
	onOpenChange(open: boolean): void;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const hash = useDocumentState((s) => s.document?.assets[assetId]?.hash);
	const [state, setState] = useState<
		| { kind: "loading" }
		| { kind: "text"; text: string }
		| { kind: "error"; code: string }
	>({ kind: "loading" });

	useEffect(() => {
		if (!open) return;
		let alive = true;
		setState({ kind: "loading" });
		runtime.client
			.inspectMedia(runtime.projectId, assetId)
			.then((result) => alive && setState({ kind: "text", text: result.text }))
			.catch(
				(error: unknown) =>
					alive &&
					setState({
						kind: "error",
						code: error instanceof ApiError ? error.code : "E_INTERNAL",
					}),
			);
		return () => {
			alive = false;
		};
	}, [open, assetId, runtime]);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{t("ui.editor.library.inspect")}</DialogTitle>
					<DialogDescription>
						<UntrustedText>{name}</UntrustedText>
					</DialogDescription>
				</DialogHeader>
				{state.kind === "loading" && (
					<p role="status" className="text-sm text-muted-foreground">
						{t("ui.editor.library.inspect_loading")}
					</p>
				)}
				{state.kind === "error" && (
					<p role="alert" className="text-sm text-destructive">
						{errorHeadline(t, state.code)}
					</p>
				)}
				{hash !== undefined && (
					<p className="text-xs">
						<span className="text-muted-foreground">
							{t("ui.editor.library.hash")}{" "}
						</span>
						<code
							data-testid="asset-hash"
							className="font-mono break-all select-all"
						>
							{hash}
						</code>
					</p>
				)}
				{state.kind === "text" && (
					<pre
						lang="en"
						data-testid="asset-details"
						className="max-h-80 overflow-auto font-mono text-xs whitespace-pre-wrap"
					>
						<UntrustedText>{state.text}</UntrustedText>
					</pre>
				)}
			</DialogContent>
		</Dialog>
	);
}
