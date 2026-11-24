import type { ReactNode } from "react";

import { GuideStrip } from "@/components/GuideStrip";
import { NumberedStep } from "@/components/NumberedStep";
import { useT } from "@/lib/i18n";

// The three things a person does first, taught the same way whatever they make: nothing here says video. The
// home and the empty list of projects show it.
export function FirstSteps(): ReactNode {
	const t = useT();
	return (
		<GuideStrip title={t("ui.dashboard.home.guide.title")}>
			{(["briefing", "write", "follow"] as const).map((step, index) => (
				<NumberedStep
					key={step}
					number={index + 1}
					title={t(`ui.dashboard.home.guide.${step}.title`)}
					description={t(`ui.dashboard.home.guide.${step}.description`)}
				/>
			))}
		</GuideStrip>
	);
}
