import { afterEach, describe, expect, test } from "vitest";

import {
	actorLabel,
	actorLabelFor,
	setUserNames,
} from "../state/activityStore";
import type { EditorBootstrap } from "./bootstrap";
import { loadUserNames } from "./userNames";

function bootstrap(mode: "local" | "network"): EditorBootstrap {
	return {
		project_id: "prj_0000000000",
		mode,
		api_base: "/api/v1",
		locale: "en",
		locales: ["en"],
		messages: {},
		user: { id: "12", name: "Ana" },
		version: "",
	};
}

const user = (id: string, email: string) => ({
	user_id: id,
	email,
	role: "member",
	disabled: false,
});

afterEach(() => setUserNames({}));

describe("loadUserNames", () => {
	test("an administrator list names the other users, the own name wins", async () => {
		await loadUserNames(bootstrap("network"), {
			listUsers: async () => ({
				users: [user("7", "bia@example.test"), user("12", "ana@example.test")],
			}),
		});
		expect(actorLabel("user:7")).toBe("bia@example.test");
		expect(actorLabel("user:12")).toBe("Ana");
		expect(actorLabel("user:9")).toBe("user:9");
	});

	test("a refused list leaves the own name and the ids of the others", async () => {
		await loadUserNames(bootstrap("network"), {
			listUsers: () => Promise.reject(new Error("E_FORBIDDEN")),
		});
		expect(actorLabel("user:12")).toBe("Ana");
		expect(actorLabel("user:7")).toBe("user:7");
	});

	test("local mode asks nothing", async () => {
		let asked = 0;
		await loadUserNames(bootstrap("local"), {
			listUsers: async () => {
				asked++;
				return { users: [] };
			},
		});
		expect(asked).toBe(0);
		expect(actorLabel("user:12")).toBe("user:12");
	});

	test("other actors keep their labels", () => {
		expect(actorLabel("local:claude-code")).toBe("claude-code");
		expect(actorLabel("key:key_ab12cd34ef")).toBe("key ab12");
	});

	test("the own actor reads as you, never as a raw id", () => {
		expect(
			actorLabelFor("user:usr_44w9jmh3vt", "user:usr_44w9jmh3vt", "you"),
		).toBe("you");
		expect(actorLabelFor("local:ui", "local:ui", "você")).toBe("você");
		expect(actorLabelFor("local:claude-code", "local:ui", "you")).toBe(
			"claude-code",
		);
	});
});
