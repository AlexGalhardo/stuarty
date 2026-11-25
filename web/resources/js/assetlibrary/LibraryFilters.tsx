import { Check } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { FilterBar } from "@/components/FilterBar";
import { OptionSelect } from "@/components/OptionSelect";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import type { AssetQuery, OriginKind } from "../editor/api/types";

const ORIGINS: readonly OriginKind[] = [
	"imported",
	"generated",
	"fetched_by_agent",
	"rendered_externally",
];

// A typed word is sent after this pause, so the service is asked once per word and not once per key.
const TYPING_PAUSE_MS = 300;

// The search and the filters of a library. They only collect what the person typed or chose and send it as a
// query, as they change: which files match is the service's answer. Every library has the same filters, so
// they are learned once (the tag is a word because the service has no list of tags to offer).
export function LibraryFilters({
	section,
	onApply,
}: {
	section: string;
	onApply(query: AssetQuery): void;
}): ReactNode {
	const t = useT();
	const [text, setText] = useState("");
	const [tag, setTag] = useState("");
	const [origin, setOrigin] = useState<OriginKind | "">("");
	const [reusable, setReusable] = useState(false);
	const first = useRef(true);
	const apply = useRef(onApply);
	apply.current = onApply;

	useEffect(() => {
		if (first.current) {
			first.current = false;
			return;
		}
		const timer = window.setTimeout(
			() =>
				apply.current({
					...(text.trim() === "" ? {} : { text: text.trim() }),
					...(tag.trim() === "" ? {} : { tag: tag.trim() }),
					...(origin === "" ? {} : { origin }),
					...(reusable ? { reusable: true } : {}),
				}),
			TYPING_PAUSE_MS,
		);
		return () => window.clearTimeout(timer);
	}, [text, tag, origin, reusable]);

	return (
		<FilterBar label={t("ui.assets.filter.bar")} className="items-end gap-3">
			<div className="flex min-w-48 flex-[2] flex-col gap-1">
				<Label htmlFor={`filter-text-${section}`}>
					{t("ui.assets.filter.text")}
				</Label>
				<Input
					id={`filter-text-${section}`}
					type="search"
					autoComplete="off"
					placeholder={t("ui.assets.filter.text_hint")}
					value={text}
					onChange={(event) => setText(event.target.value)}
				/>
			</div>
			<div className="flex min-w-36 flex-1 flex-col gap-1">
				<Label htmlFor={`filter-tag-${section}`}>
					{t("ui.assets.filter.tag")}
				</Label>
				<Input
					id={`filter-tag-${section}`}
					autoComplete="off"
					value={tag}
					onChange={(event) => setTag(event.target.value)}
				/>
			</div>
			<div className="flex min-w-44 flex-1 flex-col gap-1">
				<span className="text-sm font-medium text-text">
					{t("ui.assets.filter.origin")}
				</span>
				<OptionSelect
					label={t("ui.assets.filter.origin")}
					value={origin === "" ? "any" : origin}
					onChange={(next) =>
						setOrigin(next === "any" ? "" : (next as OriginKind))
					}
					className="w-full"
					options={[
						{ value: "any", label: t("ui.assets.filter.any_origin") },
						...ORIGINS.map((kind) => ({
							value: kind,
							label: t(`ui.assets.origin.${kind}`),
						})),
					]}
				/>
			</div>
			<Button
				type="button"
				variant="outline"
				aria-pressed={reusable}
				onClick={() => setReusable((was) => !was)}
				className="aria-pressed:border-accent aria-pressed:bg-accent-soft"
			>
				{reusable && <Check aria-hidden="true" />}
				{t("ui.assets.filter.reusable")}
			</Button>
		</FilterBar>
	);
}
