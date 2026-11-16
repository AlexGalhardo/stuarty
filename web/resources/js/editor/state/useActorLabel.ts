import { useT } from "@/lib/i18n";
import { actorLabelFor } from "./activityStore";
import { useRuntime } from "./EditorContext";

// `actorLabel` for a component: the own actor reads "you" in the language of
// the page.
export function useActorLabel(): (actor: string) => string {
	const t = useT();
	const { ownActor } = useRuntime();
	const you = t("ui.editor.activity.you");
	return (actor) => actorLabelFor(actor, ownActor, you);
}
