import { Check } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { FieldLabel } from "@/components/FieldHelp";
import { FilterBar } from "@/components/FilterBar";
import { OptionSelect } from "@/components/OptionSelect";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useT } from "@/lib/i18n";
import type {
	AssetQuery,
	OriginKind,
	SoundKind,
	TagInUse,
} from "../editor/api/types";

const ORIGINS: readonly OriginKind[] = [
	"imported",
	"generated",
	"fetched_by_agent",
	"rendered_externally",
];

const SOUND_KINDS: readonly SoundKind[] = ["music", "voice", "effect"];

// The "any tag" option cannot be a tag a person wrote.
const ANY_TAG = "any";

// A typed word is sent after this pause, so the service is asked once per word and not once per key.
const TYPING_PAUSE_MS = 300;

// The search and the filters of a library. They only collect what the person typed or chose and send it as a
// query, as they change: which files match is the service's answer. Every library has the same filters, so
// they are learned once. The tag is chosen from the tags the service says are in use, each with its count; the
// sound kind filter is only on the sounds page.
export function LibraryFilters({
	section,
	tagsInUse = [],
	onApply,
}: {
	section: string;
	tagsInUse?: readonly TagInUse[];
	onApply(query: AssetQuery): void;
}): ReactNode {
	const t = useT();
	const [text, setText] = useState("");
	const [tag, setTag] = useState("");
	const [origin, setOrigin] = useState<OriginKind | "">("");
	const [soundKind, setSoundKind] = useState<SoundKind | "">("");
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
					...(tag === "" ? {} : { tag }),
					...(soundKind === "" ? {} : { sound_kind: soundKind }),
					...(origin === "" ? {} : { origin }),
					...(reusable ? { reusable: true } : {}),
				}),
			TYPING_PAUSE_MS,
		);
		return () => window.clearTimeout(timer);
	}, [text, tag, origin, reusable, soundKind]);

	// The chosen tag stays on the list even when the other filters leave no file with it.
	const tagOptions = tagsInUse.some((entry) => entry.tag === tag)
		? tagsInUse
		: tag === ""
			? tagsInUse
			: [...tagsInUse, { tag, count: 0 }];

	return (
		<FilterBar label={t("ui.assets.filter.bar")} className="items-end gap-3">
			<div className="flex min-w-48 flex-[2] flex-col gap-1">
				<FieldLabel
					htmlFor={`filter-text-${section}`}
					label={t("ui.assets.filter.text")}
					help={t("ui.help.g3.lib_filter_help")}
				/>
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
				<span className="text-sm font-medium text-text">
					{t("ui.assets.filter.tag")}
				</span>
				<OptionSelect
					label={t("ui.assets.filter.tag")}
					value={tag === "" ? ANY_TAG : tag}
					onChange={(next) => setTag(next === ANY_TAG ? "" : next)}
					className="w-full"
					options={[
						{ value: ANY_TAG, label: t("ui.assets.filter.any_tag") },
						...tagOptions.map((entry) => ({
							value: entry.tag,
							label: t("ui.assets.filter.tag_option", {
								tag: entry.tag,
								count: entry.count,
							}),
						})),
					]}
				/>
			</div>
			{section === "audio" && (
				<div className="flex min-w-36 flex-1 flex-col gap-1">
					<span className="text-sm font-medium text-text">
						{t("ui.assets.filter.sound_kind")}
					</span>
					<OptionSelect
						label={t("ui.assets.filter.sound_kind")}
						value={soundKind === "" ? "any" : soundKind}
						onChange={(next) =>
							setSoundKind(next === "any" ? "" : (next as SoundKind))
						}
						className="w-full"
						options={[
							{ value: "any", label: t("ui.assets.filter.any_kind") },
							...SOUND_KINDS.map((kind) => ({
								value: kind,
								label: t(`ui.assets.sound.kind.${kind}`),
							})),
						]}
					/>
				</div>
			)}
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
