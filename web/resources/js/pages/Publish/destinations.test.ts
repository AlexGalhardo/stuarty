import { describe, expect, test } from "vitest";

import type {
	Connection,
	SocialAccount,
	SocialNetwork,
} from "../../editor/api/types";
import { destinationsFor } from "./destinations";

const account = (id: string, network: string): SocialAccount => ({
	connection_id: id,
	network,
	label: id,
	state: "ready",
});
const network = (name: string, publishes: string[]): SocialNetwork =>
	({ network: name, name, publishes }) as SocialNetwork;
const blog = (id: string, accepts: string[]): Connection => ({
	id,
	kind: "blog_wordpress",
	label: id,
	endpoint: "https://blog.example",
	hosts: [],
	accepts,
	created_at: "2026-11-24T00:00:00Z",
	last_used_at: null,
	secret_set: true,
});

describe("the destinations of a content kind", () => {
	const accounts = [account("yt", "youtube")];
	const networks = [network("youtube", ["video", "thumbnail"])];
	const blogs = [blog("wp", ["text", "image"])];

	test("a text post: the blog is usable and the video network is kept, disabled, with what it takes", () => {
		const list = destinationsFor("text", accounts, networks, blogs);
		expect(list.map((d) => [d.id, d.usable])).toEqual([
			["wp", true],
			["yt", false],
		]);
		expect(list[1]?.takes).toEqual(["video", "thumbnail"]);
	});

	test("a video: the network is usable and the blog is disabled", () => {
		const list = destinationsFor("video", accounts, networks, blogs);
		expect(list.map((d) => [d.id, d.usable])).toEqual([
			["yt", true],
			["wp", false],
		]);
	});

	// The service refuses a video to a blog and a post to a network at scheduling (SDD-020 section 43), whatever
	// the list of the destination says: the page never offers what the service refuses.
	test("a blog that accepts video is not offered a video, and a network that publishes text is not offered a post", () => {
		const videoBlog = [blog("wp", ["text", "video"])];
		const textNetwork = [network("youtube", ["video", "text"])];
		const forVideo = destinationsFor("video", [], networks, videoBlog);
		expect(forVideo.map((d) => [d.id, d.usable, d.blocked])).toEqual([
			["wp", false, "other_family"],
		]);
		const forText = destinationsFor("text", accounts, textNetwork, []);
		expect(forText.map((d) => [d.id, d.usable, d.blocked])).toEqual([
			["yt", false, "other_family"],
		]);
	});

	test("a blog whose list does not hold text is kept, disabled, with what it takes", () => {
		const list = destinationsFor("text", [], networks, [blog("wp", ["image"])]);
		expect(list.map((d) => [d.id, d.usable, d.blocked])).toEqual([
			["wp", false, "takes"],
		]);
	});

	test("an account that is not signed in is not offered, and a network the service does not describe takes nothing", () => {
		const list = destinationsFor(
			"video",
			[
				{ ...account("a", "x"), state: "needs_sign_in" },
				account("b", "mystery"),
			],
			networks,
			[],
		);
		expect(list.map((d) => [d.id, d.usable])).toEqual([["b", false]]);
	});
});
