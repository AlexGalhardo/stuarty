import { Fragment, type ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { keyed } from "./keyed";

// The preview of a text (SDD-019 section 11): headings, paragraphs, lists, emphasis and code, drawn from text nodes
// only. It never builds HTML from the text, so a model or an agent that wrote markup gets it shown as text: the
// element names are chosen here, the characters come from React as text. A link is shown as the text it is.

export type Block =
	| { kind: "heading"; level: number; text: string }
	| { kind: "paragraph"; text: string }
	| { kind: "list"; ordered: boolean; items: string[] }
	| { kind: "code"; text: string };

const HEADING = /^ {0,3}(#{1,6})[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$/;
const BULLET = /^ {0,3}[-*+][ \t]+(.*)$/;
const NUMBERED = /^ {0,3}\d{1,9}[.)][ \t]+(.*)$/;
const FENCE = /^ {0,3}(```|~~~)/;

// blocks reads the Markdown the way a reader does: the blocks only, no inline marks.
export function blocks(source: string): Block[] {
	const out: Block[] = [];
	const lines = source.replaceAll("\r\n", "\n").split("\n");
	let paragraph: string[] = [];
	let list: { ordered: boolean; items: string[] } | null = null;

	const flushParagraph = (): void => {
		if (paragraph.length > 0) {
			out.push({ kind: "paragraph", text: paragraph.join(" ") });
			paragraph = [];
		}
	};
	const flushList = (): void => {
		if (list !== null) {
			out.push({ kind: "list", ...list });
			list = null;
		}
	};

	for (let i = 0; i < lines.length; i += 1) {
		const line = lines[i] ?? "";
		if (FENCE.test(line)) {
			flushParagraph();
			flushList();
			const code: string[] = [];
			i += 1;
			while (i < lines.length && !FENCE.test(lines[i] ?? "")) {
				code.push(lines[i] ?? "");
				i += 1;
			}
			out.push({ kind: "code", text: code.join("\n") });
			continue;
		}
		if (line.trim() === "") {
			flushParagraph();
			flushList();
			continue;
		}
		const heading = HEADING.exec(line);
		if (heading !== null) {
			flushParagraph();
			flushList();
			out.push({
				kind: "heading",
				level: (heading[1] ?? "#").length,
				text: heading[2] ?? "",
			});
			continue;
		}
		const bullet = BULLET.exec(line);
		const numbered = bullet === null ? NUMBERED.exec(line) : null;
		const item = bullet ?? numbered;
		if (item !== null) {
			flushParagraph();
			const ordered = bullet === null;
			if (list !== null && list.ordered !== ordered) flushList();
			list ??= { ordered, items: [] };
			list.items.push(item[1] ?? "");
			continue;
		}
		flushList();
		paragraph.push(line.trim());
	}
	flushParagraph();
	flushList();
	return out;
}

const INLINE = /(`[^`]+`|\*\*[^*]+\*\*|\*[^*\s][^*]*\*|_[^_\s][^_]*_)/;

function Inline({ text }: { text: string }): ReactNode {
	return (
		<>
			{keyed(text.split(INLINE), (part) => part).map(({ key, item: part }) => {
				if (part.length > 2 && part.startsWith("`") && part.endsWith("`")) {
					return (
						<code key={key} className="rounded bg-muted px-1">
							{part.slice(1, -1)}
						</code>
					);
				}
				if (part.length > 4 && part.startsWith("**") && part.endsWith("**")) {
					return <strong key={key}>{part.slice(2, -2)}</strong>;
				}
				if (
					part.length > 2 &&
					((part.startsWith("*") && part.endsWith("*")) ||
						(part.startsWith("_") && part.endsWith("_")))
				) {
					return <em key={key}>{part.slice(1, -1)}</em>;
				}
				return <Fragment key={key}>{part}</Fragment>;
			})}
		</>
	);
}

const HEADING_CLASS: Record<number, string> = {
	1: "text-xl font-semibold",
	2: "text-lg font-semibold",
	3: "text-base font-semibold",
};

function renderBlock(block: Block, key: string): ReactNode {
	switch (block.kind) {
		case "heading":
			return (
				<p key={key} className={HEADING_CLASS[block.level] ?? "font-medium"}>
					<UntrustedText>
						<Inline text={block.text} />
					</UntrustedText>
				</p>
			);
		case "paragraph":
			return (
				<p key={key}>
					<UntrustedText>
						<Inline text={block.text} />
					</UntrustedText>
				</p>
			);
		case "list": {
			const items = keyed(block.items, (item) => item).map(
				({ key: at, item }) => (
					<li key={at}>
						<UntrustedText>
							<Inline text={item} />
						</UntrustedText>
					</li>
				),
			);
			return block.ordered ? (
				<ol key={key} className="list-decimal pl-6">
					{items}
				</ol>
			) : (
				<ul key={key} className="list-disc pl-6">
					{items}
				</ul>
			);
		}
		case "code":
			return (
				<pre
					key={key}
					className="overflow-auto rounded bg-muted p-2 whitespace-pre"
				>
					<code>{block.text}</code>
				</pre>
			);
	}
	return null;
}

// MarkdownPreview draws the blocks. A heading is a paragraph that looks like one: the page has its own outline,
// and a text with a heading of level 1 must not break it.
export default function MarkdownPreview({
	text,
	label,
	empty,
}: {
	text: string;
	label: string;
	empty: string;
}): ReactNode {
	const parts = blocks(text);
	return (
		<section
			aria-label={label}
			data-testid="write-preview"
			className="flex max-h-[32rem] flex-col gap-3 overflow-auto rounded-lg border p-3 text-sm"
			// biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable text must be reachable by keyboard
			tabIndex={0}
		>
			{parts.length === 0 && <p className="text-muted-foreground">{empty}</p>}
			{keyed(parts, (b) => `${b.kind}:${JSON.stringify(b)}`).map(
				({ key, item }) => renderBlock(item, key),
			)}
		</section>
	);
}
