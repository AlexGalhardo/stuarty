import type { ReactNode } from "react";

import AudioProcessors from "../inspector/panels/AudioProcessors";

// The inspector of the studio voice stage: the chain of the item, with the preset that makes it
// (`audio.apply_voice_preset`), each processor and its controls, the meters and the way to listen.
export default function VoiceInspector({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	return <AudioProcessors target={{ itemId }} />;
}
