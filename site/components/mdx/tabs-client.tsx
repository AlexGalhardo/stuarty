export type TabItem = { label: string; content: React.ReactNode; action?: React.ReactNode };

/**
 * Static tabs. A page script switches the visible panel; the first panel
 * is shown without JavaScript.
 */
export function TabsClient({ items, variant = "default" }: { items: TabItem[]; variant?: "default" | "code" }) {
	const code = variant === "code";
	return (
		<div
			data-tabs
			className={
				code ? "overflow-hidden rounded-xl border border-border bg-code-bg" : "rounded-xl border border-border"
			}
		>
			{/* Code tabs scroll sideways without a scrollbar and end before the copy button, which never covers a tab. */}
			<div className={code ? "border-b border-border pr-10" : "contents"}>
			<div
				role="tablist"
				className={`relative flex overflow-x-auto px-1.5 ${code ? "h-10 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden" : "gap-1 border-b border-border"}`}
				aria-label={code ? "Code examples" : "Tabs"}
			>
				{items.map((item, i) => (
					<button
						key={i}
						type="button"
						role="tab"
						data-tab={i}
						aria-selected={i === 0}
						className={`relative cursor-pointer ${code ? "px-1.5" : "px-2.5"} whitespace-nowrap text-muted transition-colors outline-none hover:text-fg focus-visible:text-fg focus-visible:underline aria-selected:text-fg ${
							code ? "font-mono text-xs" : "py-3 text-sm font-medium"
						}`}
					>
						{item.label}
					</button>
				))}
			</div>
			</div>
			{items.map((item, i) => (
				<div
					key={i}
					role="tabpanel"
					data-panel={i}
					hidden={i !== 0}
					className={code ? "relative" : "prose px-5 py-4"}
				>
					{item.action ? <div className="absolute -top-[2.3rem] right-1.5">{item.action}</div> : null}
					{item.content}
				</div>
			))}
		</div>
	);
}
