import { Link } from "@inertiajs/react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useLocale, useT } from "@/lib/i18n";
import { formatMoney } from "../../dashboard/approval/money";
import type { Job, ModelView, ProviderView } from "../../editor/api/types";
import type { ProjectSummary } from "../../types";
import {
	listModels,
	loadModelEditChoices,
	type ModelEditRequest,
	type ModelOperation,
	startModelEdit,
} from "../api/modelEdit";
import { useImage, useImageStore } from "../state/context";
import { choicesOf, modelJobStore, orderedModels } from "./state";

// The dialog of "Remove background" and "Generate" (SDD-016 section 6.4, SDD-012 section 24.14): the project that
// holds the file, the connection, the model, the prompt of a generation, the plain words that say what is sent
// and to which provider, the estimate the service made (a dry run) before anything is submitted, and then the job,
// which the strip under the toolbar follows. It computes nothing: the estimate, the limits, the waiting for a
// person and what is put in the document are the service's.

const SELECT_CLASS =
	"h-8 rounded-lg border border-input bg-background px-2 text-sm";

export function ModelEditDialog({
	operation,
	layerId,
	open,
	onOpenChange,
}: {
	operation: ModelOperation;
	// remove_background: the layer whose background goes. generate: the layer the picture may cover, or null.
	layerId: string | null;
	open: boolean;
	onOpenChange(open: boolean): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const store = useImageStore();
	const doc = useImage((s) => s.doc);
	const [projects, setProjects] = useState<ProjectSummary[] | null>(null);
	const [providers, setProviders] = useState<ProviderView[] | null>(null);
	const [project, setProject] = useState("");
	const [connection, setConnection] = useState("");
	const [models, setModels] = useState<ModelView[]>([]);
	const [model, setModel] = useState("");
	const [prompt, setPrompt] = useState("");
	const [cover, setCover] = useState(false);
	const [estimate, setEstimate] = useState<Job | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);

	const isRemoval = operation === "remove_background";
	const layer = (function find(
		layers: NonNullable<typeof doc>["layers"],
	): NonNullable<typeof doc>["layers"][number] | undefined {
		for (const l of layers) {
			if (l.id === layerId) return l;
			const inside = l.children === undefined ? undefined : find(l.children);
			if (inside !== undefined) return inside;
		}
		return undefined;
	})(doc?.layers ?? []);

	useEffect(() => {
		if (!open) return;
		let alive = true;
		setFailure(null);
		void (async () => {
			try {
				const found = await loadModelEditChoices();
				if (!alive) return;
				setProjects(found.projects);
				setProviders(found.providers);
			} catch (error) {
				if (alive) setFailure(failureOf(error));
			}
		})();
		return () => {
			alive = false;
		};
	}, [open]);

	const choices = providers === null ? [] : choicesOf(providers, operation);
	const chosen = choices.find((c) => c.connectionId === connection);

	useEffect(() => {
		if (project === "" && projects !== null && projects[0] !== undefined) {
			setProject(projects[0].id);
		}
	}, [project, projects]);

	useEffect(() => {
		if (connection === "" && choices[0] !== undefined) {
			setConnection(choices[0].connectionId);
		}
	}, [connection, choices]);

	useEffect(() => {
		if (!open || connection === "") return;
		let alive = true;
		setEstimate(null);
		void (async () => {
			try {
				const ordered = orderedModels(await listModels(connection));
				if (!alive) return;
				setModels(ordered);
				setModel(ordered[0]?.id ?? "");
			} catch (error) {
				if (alive) setFailure(failureOf(error));
			}
		})();
		return () => {
			alive = false;
		};
	}, [open, connection]);

	const request = useCallback(
		(dryRun: boolean): Promise<Job> => {
			const body: ModelEditRequest = {
				project_id: project,
				connection_id: connection,
				operation,
				model,
				...(isRemoval || cover
					? layerId === null
						? {}
						: { layer_id: layerId }
					: {}),
				...(isRemoval ? {} : { prompt }),
				...(dryRun ? { dry_run: true } : {}),
			};
			return startModelEdit(store.getState().imageId, body);
		},
		[
			project,
			connection,
			operation,
			model,
			isRemoval,
			cover,
			layerId,
			prompt,
			store,
		],
	);

	async function showEstimate(): Promise<void> {
		setFailure(null);
		setBusy(true);
		try {
			setEstimate(await request(true));
		} catch (error) {
			setEstimate(null);
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setFailure(null);
		setBusy(true);
		try {
			const job = await request(false);
			modelJobStore.getState().track({
				job,
				operation,
				imageId: store.getState().imageId,
				layerId: isRemoval || cover ? layerId : null,
				layerName: layer?.name ?? "",
			});
			onOpenChange(false);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	const ready =
		project !== "" &&
		connection !== "" &&
		model !== "" &&
		(isRemoval ? layerId !== null : prompt.trim() !== "");
	const noProvider = providers !== null && choices.length === 0;
	const providerName = chosen?.providerName ?? "";

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				className="max-h-[calc(100vh-48px)] overflow-auto sm:max-w-[560px]"
				data-testid="model-edit-dialog"
				data-operation={operation}
			>
				<DialogHeader>
					<DialogTitle>{t(`ui.image.model.title.${operation}`)}</DialogTitle>
					<DialogDescription>
						{t(`ui.image.model.intro.${operation}`)}
					</DialogDescription>
				</DialogHeader>
				{noProvider && (
					<p className="text-sm" data-testid="model-edit-no-provider">
						{t(`ui.image.model.no_provider.${operation}`)}{" "}
						<Link href="/connections" className="underline">
							{t("ui.image.model.open_connections")}
						</Link>
					</p>
				)}
				{!noProvider && (
					<form
						onSubmit={(event) => void submit(event)}
						className="flex flex-col gap-3"
					>
						<p
							className="rounded-lg border border-input bg-muted/40 px-3 py-2 text-sm"
							data-testid="model-edit-sent"
							role="note"
						>
							{isRemoval
								? t("ui.image.model.sent.remove_background", {
										layer: layer?.name ?? "",
										provider: providerName,
									})
								: t("ui.image.model.sent.generate", {
										provider: providerName,
									})}
						</p>
						<div className="flex flex-col gap-1">
							<Label htmlFor="model-edit-project">
								{t("ui.image.model.project")}
							</Label>
							<select
								id="model-edit-project"
								value={project}
								onChange={(event) => setProject(event.target.value)}
								className={SELECT_CLASS}
							>
								{(projects ?? []).map((p) => (
									<option key={p.id} value={p.id}>
										{p.name}
									</option>
								))}
							</select>
							<p className="text-xs text-muted-foreground">
								{t("ui.image.model.project_hint")}
							</p>
							{projects !== null && projects.length === 0 && (
								<p className="text-sm" data-testid="model-edit-no-project">
									{t("ui.image.model.no_project")}
								</p>
							)}
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="model-edit-connection">
								{t("ui.image.model.connection")}
							</Label>
							<select
								id="model-edit-connection"
								value={connection}
								onChange={(event) => setConnection(event.target.value)}
								className={SELECT_CLASS}
							>
								{choices.map((choice) => (
									<option key={choice.connectionId} value={choice.connectionId}>
										{choice.label}
									</option>
								))}
							</select>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="model-edit-model">
								{t("ui.image.model.model")}
							</Label>
							<select
								id="model-edit-model"
								value={model}
								onChange={(event) => {
									setModel(event.target.value);
									setEstimate(null);
								}}
								className={SELECT_CLASS}
							>
								{models.map((m) => (
									<option key={m.id} value={m.id}>
										{m.id}
										{m.price === null
											? ` (${t("ui.image.model.no_price")})`
											: ""}
									</option>
								))}
							</select>
						</div>
						{!isRemoval && (
							<>
								<div className="flex flex-col gap-1">
									<Label htmlFor="model-edit-prompt">
										{t("ui.image.model.prompt")}
									</Label>
									<Textarea
										id="model-edit-prompt"
										rows={4}
										value={prompt}
										onChange={(event) => {
											setPrompt(event.target.value);
											setEstimate(null);
										}}
									/>
								</div>
								{layerId !== null && (
									<label className="flex items-center gap-2 text-sm">
										<input
											type="checkbox"
											checked={cover}
											onChange={(event) => {
												setCover(event.target.checked);
												setEstimate(null);
											}}
										/>
										<span>
											{t("ui.image.model.cover_layer")}{" "}
											<UntrustedText>{layer?.name ?? ""}</UntrustedText>
										</span>
									</label>
								)}
							</>
						)}
						<div className="flex flex-wrap items-center gap-3">
							<Button
								type="button"
								variant="outline"
								disabled={!ready || busy}
								onClick={() => void showEstimate()}
							>
								{t("ui.image.model.estimate")}
							</Button>
							<Button type="submit" disabled={!ready || busy}>
								{t("ui.image.model.submit")}
							</Button>
						</div>
						{estimate?.estimate !== undefined && (
							<p className="text-sm" data-testid="model-edit-estimate">
								{t("ui.image.model.estimated", {
									amount: formatMoney(estimate.estimate, locale),
								})}
							</p>
						)}
					</form>
				)}
				<FailureNote failure={failure} />
			</DialogContent>
		</Dialog>
	);
}
