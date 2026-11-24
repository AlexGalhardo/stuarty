# Screen recording: gap analysis

**Status: validated by the author on 2026-11-23.** The epics are `docs/plans/61` to `73`.

Effort: S, M, L, XL (P, M, G, GG). Value is for independent creators. Competitor features are claims of its site (`research.md`). Evidence was checked in the code at `release/v0.48.0`.

## 1. Recording

| Feature | Stuarty today | Evidence | Value | Effort | Agent-first fit | Recommendation |
| --- | --- | --- | --- | --- | --- | --- |
| Screen capture | Missing | No capture API in the code; excluded by `docs/prds/001-video-editor.md` section 7 | High | XL | Agent prepares; a person starts | Do |
| Microphone and system audio | Missing | Same | High | L | Devices listed by a command | Do |
| Webcam on its own track | Missing | Same | High | M | Same core | Do |
| Pause, resume, stop, retake | Missing | Same | High | M | Commands and markers | Do |
| Cursor and click events saved | Missing | Same | High | M | Events readable by agents | Do (Linux Wayland limited) |
| Automatic zoom from clicks | Partial: manual only | `core/command/animation.go` (`keyframe.set`) | High | L | Proposal as keyframes | Do |
| Smooth cursor | Missing | No cursor layer | High | L | Same core | Do |
| Click highlight | Missing | No cursor layer | Medium | M | Same core | Do |
| Layouts and backgrounds | Partial: framing only | `core/command/clip_adjust.go` (`clip.set_framing`) | High | M | Named presets | Do |
| Teleprompter | Missing | No match | Medium | M | Fed by the Markdown script | Adapt |

## 2. AI editing

| Feature | Stuarty today | Evidence | Value | Effort | Agent-first fit | Recommendation |
| --- | --- | --- | --- | --- | --- | --- |
| Silence removal | Has | `core/command/clip_silence.go` | High | S | Already a command | Keep, offer after recording |
| Filler words and repetitions | Partial: skill guidance only | `web/resources/skills/captions-for-shorts/SKILL.md` | High | M | Proposal reviewed by a person | Do |
| Edit by text | Missing | `caption.edit_word` edits captions only | High | L | Ideal for agents | Do |
| Vertical cuts from a long video | Partial: skill, no command | `web/resources/skills/long-talking-video/SKILL.md` | High | L | Agent proposes, a person accepts | Do |
| Face-aware reframing | Partial: manual | `core/command/settings_reframe.go`, `clip.set_framing` | High | L | Same core | Do with YuNet |
| Title, description, chapters | Partial | `core/export/chapters.go`, skill `chapter-titles` | Medium | S | Already agent work | Adapt |
| Blur of sensitive data | Partial: manual | `core/command/filter.go`, `clip.set_mask` | High | M | Agent flags, a person confirms | Do |
| Highlight of an area | Partial | `clip.set_mask` | Medium | S | Named preset | Adapt |

## 3. Captions

| Feature | Stuarty today | Evidence | Value | Effort | Agent-first fit | Recommendation |
| --- | --- | --- | --- | --- | --- | --- |
| Word by word | Has | `core/command/caption_transcript.go`, `caption_words.go` | High | S | Already a command | Keep |
| Named style library | Partial: fields, no presets | No caption preset in `core/` | High | M | Named presets | Do (own styles) |
| Vertical formatting | Partial | `settings.reframe` | High | S | Same core | Adapt |
| One-click recipes (zoom, text, sound) | Missing | No match | Medium | M | Skills over commands | Adapt, as skills |

## 4. Finishing

| Feature | Stuarty today | Evidence | Value | Effort | Agent-first fit | Recommendation |
| --- | --- | --- | --- | --- | --- | --- |
| Multitrack | Has | `track.add`, `track.update` | High | n/a | Has | Keep |
| Colour, curves, LUT | Has | `clip.set_color`, `engine/lut/lut.go` | Medium | n/a | Has | Keep |
| Background removal on video | Partial: chroma key; AI on images through a provider | `core/command/chroma_key.go`, `service/providers/openai/image_edit.go` | High | L | Long job with a preview | Adapt with BEN2 base, if its licence clears |
| Studio voice | Partial | `engine/audio/denoise.go`, `audio.processor_add`, `loudness_target` | High | M | One preset | Adapt |
| Effects and transitions | Has | `clip.set_effects`, `item.set_transition` | Medium | n/a | Has | Keep |
| Keyframes | Has | `core/command/animation.go` | High | n/a | Has | Keep |
| Mixer panel | Partial: per-track controls | `track.update`, `audio.processor_add` | Medium | M | Same core | Later |
| Animated titles | Has | `text.apply_preset`, `animation.apply_preset` | Medium | n/a | Has | Keep |
| Stickers | Partial: animated image clips | `clip.add` | Low | M | Library assets | Do not do now |
| Agent editing through MCP | Has | `web/app/mcp/definitions/edit.json` | High | n/a | Core of the product | Keep |

## 5. AI tools requested by the author

No tool below is in the product today.

| Tool | Stuarty today | Evidence | Value | Effort | Where it enters the flow | Recommendation |
| --- | --- | --- | --- | --- | --- | --- |
| WhisperX | Missing, rejected | `docs/adrs/0021-transcription-providers.md:44` | Medium | L | Word alignment | Do not ship; keep whisper.cpp, measure word accuracy first |
| YuNet | Missing | No match | High | M | Vertical cuts, webcam framing | Do, required for reframing |
| BEN2 base | Missing | No match | High | L | Webcam and video background | Adapt, optional download, after the weights licence |
| MediaPipe | Missing | No match | Low | M | Landmarks for finer tracking | Only if YuNet is not enough |
| Depth-Anything-V2 Small | Missing | No match | Medium | M | Parallax, depth blur on images | Adapt, optional, later. Never Base, Large or Giant |

## 6. Opportunities the competitor does not offer

| Opportunity | Value | Effort | Note |
| --- | --- | --- | --- |
| Agent prepares a recording, a person starts it | High | M | Capture is sensitive: an agent never starts it alone, and an indicator is always visible |
| Agent marks takes and mistakes during recording | High | M | Markers in the core |
| Agent proposes cuts in the approval queue | High | L | Applied only after a person accepts |
| Teleprompter from the project's Markdown script | High | S | Reuses writing |
| Recording to published post in one reviewed flow | High | M | Reuses the publication approval |

## 7. Decisions still open

1. Amend PRD-001 section 7, which excludes recording.
2. BEN2 base: accept a weights licence with no text of its own, or ask its authors for written confirmation.
3. Helper language: Rust is proposed, which needs an exception to the rule that keeps Rust inside `renderer/`.
4. macOS: signing and notarization, since installers are unsigned today.

## 8. External motion engines

Added by an author decision of 2026-11-23, after the validation of the sections above.

| Feature | Stuarty today | Evidence | Value | Effort | Agent-first fit | Recommendation |
| --- | --- | --- | --- | --- | --- | --- |
| HyperFrames round through the hand-off | Done: real round proven with npm `hyperframes` 0.9.86 | `service/motion.go`, `service/motion_check.go`, `docs/plans/33-external-motion-engine-hand-off.md` task 7 | Medium | S | The agent renders on its side and imports | Done (epic 73) |
| Remotion | Out | ADR-0027 | Low | n/a | Importing its output still works | Do not do |
| Hosted HeyGen MCP connector | Out | ADR-0032 | Low | n/a | Would add a central outbound service | Do not do |
