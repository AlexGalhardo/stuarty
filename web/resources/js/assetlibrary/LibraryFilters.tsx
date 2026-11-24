import { type FormEvent, type ReactNode, useState } from "react";

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

// The search and the filters of a library. They only collect what the person typed and send it as a query:
// which files match is the service's answer. Every library has the same four filters, so it is learned once.
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

	function search(event: FormEvent<HTMLFormElement>): void {
		event.preventDefault();
		onApply({
			...(text.trim() === "" ? {} : { text: text.trim() }),
			...(tag.trim() === "" ? {} : { tag: tag.trim() }),
			...(origin === "" ? {} : { origin }),
			...(reusable ? { reusable: true } : {}),
		});
	}

	function clear(): void {
		setText("");
		setTag("");
		setOrigin("");
		setReusable(false);
		onApply({});
	}

	return (
		<search>
			<form
				onSubmit={search}
				className="flex flex-wrap items-end gap-3 rounded-lg border border-border bg-surface p-4"
			>
				<div className="flex min-w-48 flex-1 flex-col gap-1">
					<Label htmlFor={`filter-text-${section}`}>
						{t("ui.assets.filter.text")}
					</Label>
					<Input
						id={`filter-text-${section}`}
						autoComplete="off"
						value={text}
						onChange={(event) => setText(event.target.value)}
					/>
				</div>
				<div className="flex min-w-40 flex-col gap-1">
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
				<fieldset className="flex flex-wrap gap-x-4 gap-y-1">
					<legend className="mb-1 text-sm font-medium text-text">
						{t("ui.assets.filter.origin")}
					</legend>
					{["", ...ORIGINS].map((kind) => (
						<label
							key={kind}
							className="flex min-h-6 items-center gap-1.5 text-sm text-text"
						>
							<input
								type="radio"
								name={`filter-origin-${section}`}
								className="accent-accent"
								checked={origin === kind}
								onChange={() => setOrigin(kind as OriginKind | "")}
							/>
							{kind === ""
								? t("ui.assets.filter.any_origin")
								: t(`ui.assets.origin.${kind}`)}
						</label>
					))}
				</fieldset>
				<label className="flex min-h-9 items-center gap-2 text-sm text-text">
					<input
						type="checkbox"
						className="accent-accent"
						checked={reusable}
						onChange={(event) => setReusable(event.target.checked)}
					/>
					{t("ui.assets.filter.reusable")}
				</label>
				<div className="flex gap-2">
					<Button type="submit">{t("ui.assets.filter.apply")}</Button>
					<Button type="button" variant="outline" onClick={clear}>
						{t("ui.assets.filter.clear")}
					</Button>
				</div>
			</form>
		</search>
	);
}
