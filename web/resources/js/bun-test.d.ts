// The part of `bun:test` the unit tests use. Bun ships the runner; its type
// package (@types/bun) is a dependency nobody approved yet, so the typecheck
// sees these declarations instead. Add what a test needs here.
declare module "bun:test" {
	interface Matchers<R = void> {
		not: Matchers<R>;
		toBe(expected: unknown): R;
		toEqual(expected: unknown): R;
		toStrictEqual(expected: unknown): R;
		toBeUndefined(): R;
		toBeDefined(): R;
		toBeNull(): R;
		toBeTruthy(): R;
		toBeFalsy(): R;
		toBeGreaterThan(expected: number): R;
		toBeGreaterThanOrEqual(expected: number): R;
		toBeLessThan(expected: number): R;
		toBeLessThanOrEqual(expected: number): R;
		toBeCloseTo(expected: number, digits?: number): R;
		toHaveLength(expected: number): R;
		toContain(expected: unknown): R;
		toMatch(expected: string | RegExp): R;
		toThrow(expected?: string | RegExp | Error): R;
		toBeInstanceOf(expected: unknown): R;
	}
	export function expect<T>(actual: T): Matchers;
	export function describe(name: string, body: () => void): void;
	export function test(
		name: string,
		body: () => void | Promise<void>,
		timeoutMs?: number,
	): void;
	export function it(
		name: string,
		body: () => void | Promise<void>,
		timeoutMs?: number,
	): void;
	export function beforeEach(body: () => void | Promise<void>): void;
	export function afterEach(body: () => void | Promise<void>): void;
}
