import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import type { ErrorLike } from "../ui/errorText";
import { useErrorText } from "./useErrorText";

// The message and the hint of an error: the catalogue wording when there is
// one, else the core's English as received, marked `lang="en"` (VV-126).
export default function ErrorBody({
	error,
	hintPrefix,
	hintClassName = "text-muted-foreground",
	className,
}: {
	error: ErrorLike;
	hintPrefix?: string;
	hintClassName?: string;
	className?: string;
}): ReactNode {
	const text = useErrorText()(error);
	const lang = text.localized ? undefined : "en";
	return (
		<>
			{text.message !== "" && (
				<p lang={lang} className={className}>
					<UntrustedText>{text.message}</UntrustedText>
				</p>
			)}
			{text.hint !== undefined && (
				<p lang={lang} className={hintClassName}>
					{hintPrefix !== undefined && `${hintPrefix} `}
					<UntrustedText>{text.hint}</UntrustedText>
				</p>
			)}
		</>
	);
}
