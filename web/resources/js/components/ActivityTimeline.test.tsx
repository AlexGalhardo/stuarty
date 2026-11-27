import { describe, expect, test, vi } from "vitest";

import type { ActivityEntry } from "@/editor/api/types";
import { drawForTest } from "@/workflow/drawForTest";
import { ActivityTimeline } from "./ActivityTimeline";

vi.mock("@inertiajs/react", () => ({
	Link: ({
		children,
		href,
		className,
	}: {
		children: unknown;
		href: string;
		className?: string;
	}) => (
		<a href={href} className={className}>
			{children as string}
		</a>
	),
}));

const sampleEntries: ActivityEntry[] = [
	{
		id: "audit::2:prj_alpha00001",
		at: "2026-11-27T14:20:00Z",
		actor: "local:chat-claude",
		actor_kind: "agent",
		actor_label: "claude",
		domain: "project",
		action: "project.update",
		project_id: "prj_alpha00001",
		project_name: "Launch Trailer",
	},
	{
		id: "audit::1:prj_alpha00001",
		at: "2026-11-27T14:10:00Z",
		actor: "local:ui",
		actor_kind: "human",
		actor_label: "local:ui",
		domain: "project",
		action: "project.create",
		project_id: "prj_alpha00001",
		project_name: "Launch Trailer",
	},
];

describe("ActivityTimeline", () => {
	test("renders localized entries with human and agent badges in pt-BR", () => {
		const html = drawForTest(
			<ActivityTimeline initialEntries={sampleEntries} />,
			"pt-BR",
		);
		expect(html).toContain("Projeto criado");
		expect(html).toContain("Projeto atualizado");
		expect(html).toContain("Agentes");
		expect(html).toContain("Você");
		expect(html).toContain("Launch Trailer");
		expect(html).toContain('href="/projects/prj_alpha00001"');
		expect(html).toContain('data-testid="history-filter-project"');
	});

	test("hides the project filter and project links when scoped to one project", () => {
		const html = drawForTest(
			<ActivityTimeline
				projectId="prj_alpha00001"
				initialEntries={sampleEntries}
			/>,
			"en",
		);
		expect(html).toContain('data-testid="project-activity-timeline"');
		expect(html).not.toContain('data-testid="history-filter-project"');
		expect(html).not.toContain('href="/projects/prj_alpha00001"');
		expect(html).toContain("Project created");
		expect(html).toContain("Project updated");
	});

	test("renders an empty state when there are no matching entries", () => {
		const html = drawForTest(<ActivityTimeline initialEntries={[]} />, "pt-BR");
		expect(html).toContain('data-testid="history-empty"');
		expect(html).toContain("Nenhuma atividade registrada");
	});

	test("localizes job outcomes and puts the internal identifier in the details", () => {
		const entry: ActivityEntry = {
			...sampleEntries[0],
			domain: "image",
			action: "job.image.generate.succeeded",
			subject_id: "job_123",
		};
		const html = drawForTest(
			<ActivityTimeline initialEntries={[entry]} />,
			"pt-BR",
		);
		expect(html).toContain("Tarefa concluída");
		expect(html).toMatch(
			/<details[^>]*>.*Detalhes da ação.*job_123.*<\/details>/,
		);
	});
});
