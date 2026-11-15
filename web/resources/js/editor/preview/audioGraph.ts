// The output stage of the preview audio (SDD-012 section 11.5): every pooled
// element feeds its own GainNode, and those feed one master GainNode before the
// destination, so the volume and the mute of the preview are one value. They
// are screen state; the core never sees them and an export is not affected.
export class AudioGraph {
	private master: GainNode | null = null;
	private volume = 1;
	private muted = false;

	attach(context: AudioContext): void {
		const master = context.createGain();
		master.connect(context.destination);
		this.master = master;
		this.apply();
	}

	detach(): void {
		this.master?.disconnect();
		this.master = null;
	}

	// Where the element gains connect; null until the context exists.
	get input(): AudioNode | null {
		return this.master;
	}

	setLevel(volume: number, muted: boolean): void {
		this.volume = Math.min(Math.max(volume, 0), 1);
		this.muted = muted;
		this.apply();
	}

	private apply(): void {
		if (this.master !== null) {
			this.master.gain.value = this.muted ? 0 : this.volume;
		}
	}
}
