---
name: thumbnail
description: Make the cover of a short or a long video in Stuarty - a still image with a big readable title, one strong picture and the brand colours, built as an image document and exported as a file. Use it after the cut, when the video needs a cover.
---

# Thumbnail

A cover has one job: make a person who scrolls stop and read it in a second. One picture, a few big words, strong contrast, nothing else.

## Steps

1. Read the brief and the script: `brief:show --project <project id>`. The cover repeats the promise of the video, not its whole content.
2. Choose the size by where it is used: `vertical_cover` (1080 by 1920) for a short on a vertical feed, `video_thumbnail` (1280 by 720) for a horizontal video, `square_post` (1080 by 1080) for a feed post. `image:create --preset vertical_cover --name "<name>"`. In MCP: the `image` tool with `action` `create` and the `preset`. When the workspace has a cover template (`image:list --templates only`), start from it instead: `image:create --from-template <template id> --name "<name>" --values -` with `{"<slot>":{"text":"<title>"}}` on standard input, then go to step 5.
3. Choose the picture. A frame of the video: look with `view --project <project id> --mode sheet --count 9`; the best frame has a face or one clear object, and the subject is not near the edges. Write that frame to a file outside the workspace with `export --project <project id> --format png --at <time> --out <absolute path>.png` (the import refuses a file inside the workspace with `E_PATH_NOT_ALLOWED`), then bring it in with `media:import <absolute path>.png --project <project id> --wait`; the import gives the `hash` to use below as the `asset`. The person's own photo works the same way.
4. Build the image with `image:edit <image id> <operation> k=v...`. The operations are listed by `command:help --domain image`, and one is described by `command:help --domain image <operation>` (without the domain its name is not found). In MCP: `image` with `action` `edit` and `operations`. The usual order:
   1. `layer.add_image` with the `asset` of the picture, fitted with `fit` cover.
   2. `layer.add_shape` for a band or a box that gives the text contrast.
   3. `layer.add_text` for the title: three to five words, a `style` with a bold font, a large size and a light colour on a dark shape, or the other way round.
   4. `layer.align` and `layer.fit` to place and size what you added.
5. Look at the result and fix it; then read the title at a small size, as on a phone: if you cannot read it at a glance, shorten it or make it bigger.
6. Export: `image:export <image id> --format png`. In MCP: `image` with `action` `export`. Tell the person where the file is.

## Rules

- At most five words of text. The title does not repeat the first line of the video; it promises the same thing in other words.
- Keep the face and the title inside the middle of the image, clear of the edges and of the corners where the player draws its own marks.
- Use the colours and the font of the brand kit when the workspace has one: `brand:show`.
- Do not use a picture the person has no right to use. `fetch-clip` records the source of a clip.
- Do not put a claim on the cover that the video does not keep.

A publication (it waits in the approval queue for a person) takes as its cover the id of a still export of the video: `export --project <project id> --format png --at <time> --wait` and its `job_id` goes in `publication:create --thumbnail`. A cover made as an image document cannot be attached yet: hand the person its file.
