import { Component, type ErrorInfo, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { ApiError } from "../api/errors";
import { type Failure, FailureView } from "../ui/FailureView";

export function failureOf(error: unknown): Failure {
	if (error instanceof ApiError) {
		return {
			code: error.code,
			message: error.message,
			...(error.hint === undefined ? {} : { hint: error.hint }),
		};
	}
	return { message: error instanceof Error ? error.message : String(error) };
}

// The whole-page view of an error that stops the editor.
export function FailurePage({ failure }: { failure: Failure }): ReactNode {
	const t = useT();
	return (
		<main className="flex h-dvh flex-col items-center justify-center gap-4 bg-ed-bg p-6 text-foreground">
			<h1 className="text-lg font-semibold">{t("ui.editor.failure.title")}</h1>
			<div
				role="alert"
				className="max-w-xl rounded-md border border-destructive p-3"
			>
				<FailureView failure={failure} />
			</div>
			<div className="flex gap-2">
				<Button type="button" onClick={() => location.reload()}>
					{t("ui.editor.banner.reload")}
				</Button>
				<Button asChild variant="outline">
					<a href="/projects">{t("ui.editor.back")}</a>
				</Button>
			</div>
		</main>
	);
}

interface State {
	failure: Failure | null;
}

// Catches what a component throws while rendering, so a defect shows its
// message and does not leave the page blank.
export class ErrorBoundary extends Component<{ children?: ReactNode }, State> {
	state: State = { failure: null };

	static getDerivedStateFromError(error: unknown): State {
		return { failure: failureOf(error) };
	}

	componentDidCatch(error: unknown, info: ErrorInfo): void {
		console.error(error, info.componentStack);
	}

	render(): ReactNode {
		return this.state.failure === null ? (
			this.props.children
		) : (
			<FailurePage failure={this.state.failure} />
		);
	}
}
