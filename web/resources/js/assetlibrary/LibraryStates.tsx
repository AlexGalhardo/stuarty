import { Link } from "@inertiajs/react";
import { AudioLines, Film, Images } from "lucide-react";
import type { ReactNode } from "react";

import { EmptyState } from "@/components/EmptyState";
import { GuideStrip } from "@/components/GuideStrip";
import { NumberedStep } from "@/components/NumberedStep";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useT } from "@/lib/i18n";

type Section = "visual" | "video" | "audio";

const ICON = { visual: Images, video: Film, audio: AudioLines } as const;

// A library that holds nothing yet: what it is for and three steps, never a blank region. The words speak
// of files, not of videos in every project: a project can be only text (D-134).
export function LibraryEmpty({ section }: { section: Section }): ReactNode {
	const t = useT();
	return (
		<EmptyState
			level={3}
			icon={ICON[section]}
			title={t(`ui.assets.empty_title.${section}`)}
			description={t(`ui.assets.empty.${section}`)}
			className="mt-4 items-start text-left"
		>
			<GuideStrip title={t("ui.assets.guide.title")} className="w-full">
				<NumberedStep
					number={1}
					title={t("ui.assets.guide.bring.title")}
					description={t("ui.assets.guide.bring.text")}
				/>
				<NumberedStep
					number={2}
					title={t("ui.assets.guide.tag.title")}
					description={t("ui.assets.guide.tag.text")}
				/>
				<NumberedStep
					number={3}
					title={t("ui.assets.guide.use.title")}
					description={t("ui.assets.guide.use.text")}
				/>
			</GuideStrip>
			<Button asChild>
				<Link href="/projects">{t("ui.assets.guide.open_projects")}</Link>
			</Button>
		</EmptyState>
	);
}

// A static skeleton of the grid and one polite sentence, with no spinner (design-system section 2.13).
export function LibraryLoading({ section }: { section: Section }): ReactNode {
	const t = useT();
	return (
		<div
			data-slot="loading-state"
			data-testid="asset-loading"
			role="status"
			className="mt-3 flex flex-col gap-3"
		>
			<p className="text-sm text-text-muted">
				{t(`ui.assets.loading.${section}`)}
			</p>
			<div
				aria-hidden="true"
				className={
					section === "audio"
						? "flex flex-col gap-3"
						: "grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-3"
				}
			>
				{[0, 1, 2, 3, 4, 5, 6, 7]
					.slice(0, section === "audio" ? 4 : 8)
					.map((n) =>
						section === "audio" ? (
							<Skeleton key={n} className="h-24 w-full" />
						) : (
							<div key={n} className="flex flex-col gap-2">
								<Skeleton className="aspect-video w-full" />
								<Skeleton className="h-3 w-full" />
								<Skeleton className="h-3 w-1/3" />
							</div>
						),
					)}
			</div>
		</div>
	);
}

// The library could not be read: what happened is in the note above (the service's code, message and hint),
// and here are the two things to do. The files are not touched by a failed read.
export function LibraryReadFailed({
	section,
	onRetry,
}: {
	section: Section;
	onRetry(): void;
}): ReactNode {
	const t = useT();
	return (
		<div className="mt-3 flex flex-col gap-4" data-testid="asset-read-failed">
			<div>
				<Button type="button" variant="outline" onClick={onRetry}>
					{t("ui.assets.error.retry")}
				</Button>
			</div>
			<GuideStrip title={t(`ui.assets.error.how.${section}`)}>
				<NumberedStep
					number={1}
					title={t("ui.assets.error.step1.title")}
					description={t("ui.assets.error.step1.text")}
				/>
				<NumberedStep
					number={2}
					title={t("ui.assets.error.step2.title")}
					description={t("ui.assets.error.step2.text")}
				/>
			</GuideStrip>
		</div>
	);
}
