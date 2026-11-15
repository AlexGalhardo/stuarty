// A bounded map that drops the least recently used entry (SDD-012 section 19
// "bounded caches").
export class Lru<K, V> {
	private readonly map = new Map<K, V>();

	constructor(
		private readonly max: number,
		private readonly onEvict?: (key: K, value: V) => void,
	) {}

	get size(): number {
		return this.map.size;
	}

	get(key: K): V | undefined {
		const value = this.map.get(key);
		if (value === undefined) return undefined;
		this.map.delete(key);
		this.map.set(key, value);
		return value;
	}

	set(key: K, value: V): void {
		this.map.delete(key);
		this.map.set(key, value);
		while (this.map.size > this.max) {
			const oldest = this.map.entries().next();
			if (oldest.done) break;
			const [k, v] = oldest.value;
			this.map.delete(k);
			this.onEvict?.(k, v);
		}
	}

	delete(key: K): void {
		this.map.delete(key);
	}

	clear(): void {
		if (this.onEvict) for (const [k, v] of this.map) this.onEvict(k, v);
		this.map.clear();
	}
}
