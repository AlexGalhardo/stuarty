import { Link } from "@inertiajs/react";
import { type ReactNode, useCallback, useEffect, useState } from "react";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	Job,
	PublishRequest,
	PublishValidateResult,
	SocialAccount,
	SocialNetwork,
} from "../../editor/api/types";

// What a network allows is what the service says in its limits; these are the choices when it said nothing.
const VISIBILITIES = ["private", "unlisted", "public"];
const STILLS = ["png", "jpg", "jpeg"];

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

function exportsOf(jobs: Job[], formats: string[] | null): Job[] {
	return jobs.filter((job) => {
		if (job.kind !== "export" || job.state !== "succeeded") return false;
		const format = job.result?.export?.format ?? "mp4";
		return formats === null ? format === "mp4" : formats.includes(format);
	});
}

// The "publish now" dialog (SDD-020 section 13). It sends the post to the service, which checks it
// against the limits of the network; a valid post becomes a job that waits in the approval queue, and
// nothing leaves this computer until a person approves it there. The dialog validates nothing itself.
export function PublishDialog({
	account,
	network,
	onClose,
}: {
	account: SocialAccount;
	network?: SocialNetwork;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	// The dialog asks only what the network takes: the visibilities it lists, the kids question when it asks
	// one, and a thumbnail when it takes one (SDD-020 section 18).
	const visibilities = network?.limits.visibilities ?? VISIBILITIES;
	const asksKids = network?.limits.needs_kids_declaration ?? true;
	// The service sends null for a network that takes no thumbnail: a known network with none says so.
	const takesThumbnail =
		network === undefined ||
		(network.limits.thumbnail_formats ?? []).length > 0;
	const [projects, setProjects] = useState<ProjectSummary[]>([]);
	const [jobs, setJobs] = useState<Job[]>([]);
	const [project, setProject] = useState("");
	const [video, setVideo] = useState("");
	const [still, setStill] = useState("");
	const [title, setTitle] = useState("");
	const [description, setDescription] = useState("");
	const [tags, setTags] = useState("");
	const [visibility, setVisibility] = useState(
		visibilities.includes("private")
			? "private"
			: (visibilities[0] ?? "private"),
	);
	const [kids, setKids] = useState<"" | "yes" | "no">("");
	const [synthetic, setSynthetic] = useState(false);
	const [checked, setChecked] = useState<PublishValidateResult | null>(null);
	const [waiting, setWaiting] = useState<Job | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);

	useEffect(() => {
		api()
			.listActiveProjects()
			.then((result) => setProjects(result.projects))
			.catch((error) => setFailure(failureOf(error)));
	}, []);

	const pick = useCallback(async (id: string): Promise<void> => {
		setProject(id);
		setVideo("");
		setStill("");
		setJobs([]);
		if (id === "") return;
		try {
			setJobs((await api().listJobs(id)).jobs);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}, []);

	function request(): PublishRequest {
		const list = tags
			.split(",")
			.map((tag) => tag.trim())
			.filter((tag) => tag !== "");
		return {
			project_id: project,
			connection_id: account.connection_id,
			export: video,
			...(still === "" || !takesThumbnail ? {} : { thumbnail: still }),
			title,
			description,
			...(list.length === 0 ? {} : { tags: list }),
			visibility,
			...(kids === "" || !asksKids ? {} : { made_for_kids: kids === "yes" }),
			ai_disclosure: synthetic,
		};
	}

	async function run(action: "check" | "send"): Promise<void> {
		setBusy(true);
		setFailure(null);
		try {
			if (action === "check") {
				setChecked(await api().validatePublish(request()));
			} else {
				setWaiting(await api().publishNow(request()));
			}
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	const ready = project !== "" && video !== "" && title.trim() !== "";
	const videos = exportsOf(jobs, null);
	const stills = exportsOf(jobs, STILLS);

	return (
		<Dialog open onOpenChange={(open) => !open && onClose()}>
			<DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
				<DialogHeader>
					<DialogTitle>{t("ui.social.publish_title")}</DialogTitle>
					<DialogDescription>
						{t("ui.social.publish_intro")}{" "}
						<UntrustedText>
							{account.account_title ?? account.label}
						</UntrustedText>
					</DialogDescription>
				</DialogHeader>
				{waiting !== null ? (
					<div
						role="status"
						className="flex flex-col gap-3 text-sm"
						data-waiting=""
					>
						<p className="font-medium">{t("ui.social.waiting_title")}</p>
						<p>{t("ui.social.waiting_body")}</p>
						<p>
							<Link href="/approvals" className="underline underline-offset-4">
								{t("ui.social.open_approvals")}
							</Link>
						</p>
						<div>
							<Button type="button" variant="outline" onClick={onClose}>
								{t("ui.social.close")}
							</Button>
						</div>
					</div>
				) : (
					<form
						className="flex flex-col gap-4"
						onSubmit={(event) => {
							event.preventDefault();
							void run("send");
						}}
					>
						<div className="flex flex-col gap-2">
							<Label htmlFor="publish-project">{t("ui.social.project")}</Label>
							<select
								id="publish-project"
								className="h-8 rounded-lg border border-input bg-transparent px-2 text-sm"
								value={project}
								onChange={(event) => void pick(event.target.value)}
							>
								<option value="">{t("ui.social.choose")}</option>
								{projects.map((p) => (
									<option key={p.id} value={p.id}>
										{p.name}
									</option>
								))}
							</select>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="publish-video">{t("ui.social.video")}</Label>
							<select
								id="publish-video"
								className="h-8 rounded-lg border border-input bg-transparent px-2 text-sm"
								value={video}
								onChange={(event) => setVideo(event.target.value)}
							>
								<option value="">{t("ui.social.choose")}</option>
								{videos.map((job) => (
									<option key={job.job_id} value={job.job_id}>
										{job.job_id} · {formatDateTime(job.created_at, locale)}
									</option>
								))}
							</select>
						</div>
						{takesThumbnail && (
							<div className="flex flex-col gap-2">
								<Label htmlFor="publish-still">
									{t("ui.social.thumbnail")}
								</Label>
								<select
									id="publish-still"
									className="h-8 rounded-lg border border-input bg-transparent px-2 text-sm"
									value={still}
									onChange={(event) => setStill(event.target.value)}
								>
									<option value="">{t("ui.social.no_thumbnail")}</option>
									{stills.map((job) => (
										<option key={job.job_id} value={job.job_id}>
											{job.job_id} · {formatDateTime(job.created_at, locale)}
										</option>
									))}
								</select>
							</div>
						)}
						<div className="flex flex-col gap-2">
							<Label htmlFor="publish-title">{t("ui.social.post_title")}</Label>
							<Input
								id="publish-title"
								required
								autoComplete="off"
								value={title}
								onChange={(event) => setTitle(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="publish-description">
								{t("ui.social.description")}
							</Label>
							<Textarea
								id="publish-description"
								value={description}
								onChange={(event) => setDescription(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="publish-tags">{t("ui.social.tags")}</Label>
							<Input
								id="publish-tags"
								autoComplete="off"
								value={tags}
								onChange={(event) => setTags(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="publish-visibility">
								{t("ui.social.visibility")}
							</Label>
							<select
								id="publish-visibility"
								className="h-8 rounded-lg border border-input bg-transparent px-2 text-sm"
								value={visibility}
								onChange={(event) => setVisibility(event.target.value)}
							>
								{visibilities.map((v) => (
									<option key={v} value={v}>
										{t(`ui.social.visibility_${v}`)}
									</option>
								))}
							</select>
						</div>
						{asksKids && (
							<fieldset className="flex flex-col gap-2">
								<legend className="text-sm font-medium">
									{t("ui.social.kids")}
								</legend>
								<div className="flex gap-4 text-sm">
									{(["yes", "no"] as const).map((answer) => (
										<label key={answer} className="flex items-center gap-2">
											<input
												type="radio"
												name="publish-kids"
												className="accent-primary"
												checked={kids === answer}
												onChange={() => setKids(answer)}
											/>
											{t(`ui.social.kids_${answer}`)}
										</label>
									))}
								</div>
							</fieldset>
						)}
						<label className="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								className="accent-primary"
								checked={synthetic}
								onChange={(event) => setSynthetic(event.target.checked)}
							/>
							{t("ui.social.synthetic")}
						</label>
						<FailureNote failure={failure} />
						{checked !== null && (
							<div
								role="status"
								className="rounded-lg border bg-muted p-3 text-sm"
								data-checked=""
							>
								{checked.state === "prepared" ? (
									<>
										<p className="font-medium">{t("ui.social.check_ok")}</p>
										{checked.summary !== undefined && (
											<p className="mt-1 break-words">
												<UntrustedText>{checked.summary}</UntrustedText>
											</p>
										)}
									</>
								) : (
									<>
										<p className="font-medium">{t("ui.social.check_issues")}</p>
										<ul className="mt-1 list-disc pl-5">
											{checked.issues.map((issue) => (
												<li key={`${issue.path}:${issue.code}`}>
													<UntrustedText>{issue.message}</UntrustedText>
												</li>
											))}
										</ul>
									</>
								)}
							</div>
						)}
						<div className="flex flex-wrap gap-2">
							<Button
								type="button"
								variant="outline"
								disabled={busy || !ready}
								onClick={() => void run("check")}
							>
								{t("ui.social.check")}
							</Button>
							<Button type="submit" disabled={busy || !ready}>
								{t("ui.social.send")}
							</Button>
							<Button type="button" variant="outline" onClick={onClose}>
								{t("ui.social.close")}
							</Button>
						</div>
					</form>
				)}
			</DialogContent>
		</Dialog>
	);
}
