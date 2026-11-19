---
name: caption-per-network
description: Write the text that goes with a finished video on each network - the post caption, hashtags and first line - inside the limits of YouTube, TikTok, Instagram, Facebook, LinkedIn and X, in the tone of the brief. Use it after the export, before the person publishes.
license: MIT
metadata:
  stuarty-source: coreyhaines31/marketingskills skills/social/SKILL.md and skills/social/references/platform-limits.md @ 73573e94a06a39965978b3fa1f9c1eace8cebc7c
  stuarty-adapted-from: marketingskills
---

# Caption per network

The words under a video are not the captions on it. Each network shows a different amount of text before it cuts the rest, allows a different number of hashtags and rewards a different way of writing. This skill is adapted from the social content skill and the platform limits reference of the marketing skills set named above (MIT licence); it is rewritten for Stuarty. The limits are the ones the table of the core recorded, with the date it was read; they change, so check the network before you rely on a limit that matters.

## Steps

1. Read the brief and the script: `brief:show --project <project id>`. The platforms to write for come from the brief; write for no network the person did not name.
2. Write the first line for the viewer who reads only that: the hook of the video in other words, not a copy of it.
3. Write one text per network, in the language of the video, as one `caption` text of the project: the words every network shares first, then a part for each network that starts with a line `## youtube`, `## instagram`, `## facebook`, `## x` or `## tiktok`. Keep it with `text:write --project <project id> --title "Caption" --kind caption --text <file|->`; in MCP, `project_item` with `action` `text`, `text_kind` `caption`, a `title` and the `text`. How much is shown before "more", and which tone each network rewards, is below.

   | Network | Shown before "more" | Hashtags that work |
   | --- | --- | --- |
   | Instagram | about 125 characters | at most 5 |
   | TikTok | about 150 | at most 5 |
   | YouTube Shorts | about 100; the title about 70 | three to five |
   | Facebook | short is better: 40 to 80 characters do well | one or two |
   | LinkedIn | about 210 | three to five |
   | X | all of it | one or two |

4. Fit each text to its network:
   - Everything the viewer must read goes before the cut. Put the link or the call to action there too.
   - Short paragraphs written the way a person would say them. No line break after every sentence.
   - Hashtags at the end, relevant to the video, never more than the table allows.
   - On LinkedIn and X, a plain statement of the point; on Instagram and TikTok, closer and more casual; on YouTube, a title that says what the video is about and a description that says why to watch.
5. Let the core measure it: `caption:validate <item id> --project <project id>`. In MCP: `project_item` with `action` `check` and the `item_id`. The answer gives, for each network, the length in the unit that network counts, the hashtags, the mentions and the links, and says which limit a text breaks and by how much. Fix every problem it names; the limits are in the table of the core, with the date each was read, and LinkedIn is not in it yet, so check that one by hand. Then check each text against the list in `write-a-hook` of machine-sounding phrases.
6. Give the person the texts, one block per network, with the count of each from the core, and say which are ready to paste.

Not available yet: placing these texts in a publication that waits in the approval queue is added by the publishing epics. Until then the person copies them.

## Rules

- Do not invent a result, a number, a quote or a name. Everything comes from the brief, the script or the person.
- Do not write a text for an account or a network the person has not named, and do not ask for a login to any of them.
- A claim in the text must be true in the video.
- Never write "link in bio" unless the person says the bio has the link.
