---
name: short-script
description: Write the script of a short vertical video, timed to the duration of the brief - hook, body beats, call to action, and what the viewer sees and hears at each moment - and keep it as a text item of the project. Use it after the hook is chosen and before the cut.
license: MIT
metadata:
  stuarty-source: coreyhaines31/marketingskills skills/social/references/short-form-video.md @ 73573e94a06a39965978b3fa1f9c1eace8cebc7c
  stuarty-adapted-from: marketingskills
---

# Short script

A script for a short is a list of timed beats, not an essay. Each beat says what the viewer sees, what they hear and what text is on the screen. This skill is adapted from the short-form video reference of the marketing skills set named above (MIT licence); the structures and the template are rewritten for Stuarty.

## Steps

1. Read the brief: `brief:show --project <project id>`. You need the platform, the duration, the tone, what must appear and what must not. If one is missing, run `brief-a-production`.
2. Take the chosen hook from `write-a-hook`, or write it first.
3. Pick the structure that fits the brief:
   - Tip or how-to, 20 to 45 seconds: hook (0 to 3 s), the point, the steps or the proof, a call to action.
   - Story, 45 to 60 seconds: hook that teases the result (0 to 3 s), set-up and stakes (to 15 s), what happened (to 45 s), the result (to 55 s), the lesson and the call to action.
   - Scene or situation, 15 to 30 seconds: a text on screen sets the scene (0 to 3 s), the scene plays out, a twist or a punchline at the end.
4. Write the script with this template, one block per beat, with times that add up to the duration of the brief:

   ```text
   Hook (0-3 s)
   - Sees: ...
   - Hears: ...
   - Text on screen: ...
   Body (3-X s)
   - [time] sees / hears / text
   Call to action (last 3-5 s)
   - Says: ... Text: ...
   Notes
   - Footage needed: ... Sound: ...
   ```

5. Write it so that the core can read it: a heading line (`## Hook`) starts a scene, a paragraph that starts with `VO:` is what is said, one that starts with `ON SCREEN:` is the text on the screen, and any other paragraph is a note for the editor.
6. Keep it with the project as a script: `text:write --project <project id> --title "Script" --kind script --text <file|->`. In MCP: `project_item` with `action` `text`, a `title`, `text_kind` `script` and the `text`. To change it later, write on top of the revision you read: `text:write <item id> --project <project id> --text <file|-> --base <revision>` (in MCP, `revision` is the one you read). Every save is a revision, and `text:revert` goes back to one.
7. Ask the core how long it takes: `script:estimate <item id> --project <project id> --wpm 150`. In MCP: `project_item` with `action` `check`, the `item_id` and `wpm`. Cut until the time fits the duration of the brief with room to breathe, and until every sentence earns its place. One idea for each beat.
8. Show the person the script and wait for their yes before the cut.

## Rules

- The video must work with the sound off: every beat that matters has text on the screen.
- The call to action asks for one thing the brief names. Not three.
- Do not put in the script a fact, a number or a claim that the brief and the person did not give.
- Do not use machine-sounding phrases (see `write-a-hook`).
- The footage you list must exist or be marked as still to shoot; a script that depends on footage nobody has is not finished.
