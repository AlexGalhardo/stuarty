# Screen reader pass: the script for the author

Epic 41, SDD-012 section 18.10. An agent cannot listen to a screen reader, so the accessibility tree assertions of the specs are its evidence, and this pass is a person's. It follows the first flow of the product (brief, edit, thumbnail, schedule, approve) with NVDA on Windows.

## Outcome

| Field | Value |
| --- | --- |
| State | **not verified** |
| Done by | nobody yet |
| Date | |
| Stuarty version | |
| NVDA version and browser | |
| Steps that failed (numbers) | |

Change the state to `verified` only after a person did every step below with the screen reader on and wrote the date, the versions and the failed steps (none, or the numbers). A step that fails is a defect: add a `VV` entry with the step number, what NVDA said and what it should have said. Nothing in this file may say `verified` until that happened.

## Before you start

1. Install NVDA (free, nvaccess.org) and start it. In its settings keep the defaults, speech on, and turn on "Speak typed words" only if you want it.
2. Start Stuarty in the browser (`stuarty serve`), with a workspace you do not mind (set `STUARTY_HOME` to a new folder). Open the address that the command prints.
3. Keep the mouse out of reach. Every step is with the keyboard: `Tab`, `Shift+Tab`, arrows, `Enter`, `Space`, `Esc`, and the NVDA keys (`Insert` is the NVDA key; `NVDA+Space` toggles browse and focus mode).
4. Use the browser at 100% zoom, in English. Repeat steps 1 to 3 in Portuguese (language control in the sidebar) when you have time; the sentences differ, the roles do not.

## The steps

Each step says what to do and what you must hear. "Hear" is a paraphrase: the words may differ a little, the role and the name may not.

| # | Do | You must hear |
| --- | --- | --- |
| 1 | Load the home page. Press `Tab` once. | A link "Skip to the main content". `Enter` moves to the page; NVDA reads the main region |
| 2 | `Tab` to the navigation. Walk the links with `Tab`. | "Main navigation", then each section as a link ("Projects, link"), and "current page" on the one you are on |
| 3 | Activate "Projects". | The page name, "Projects, heading level 1", and the focus is at the top of the main region |
| 4 | `Tab` to the field "Name", type a name, `Tab` to the button, `Enter`. | "Name, edit"; "Create project, button"; the editor opens with its toolbars named |
| 5 | In the editor press `F6` repeatedly. | The regions in turn: top bar, library, preview, timeline, inspector, each by name |
| 6 | In the library, `Tab` to "Path of a file to import", type the path of a video, `Enter` on "Import". Then "Add to timeline". | The asset card is read with its name, kind and duration; after adding, "Moved, 1 item" is not said, but the clip appears as a cell of the timeline |
| 7 | `F6` to the timeline. Arrow to the clip. Press `Enter`. | "Selected the name, Clip, 00:00.000 to 00:05.000, track Video 1" |
| 8 | Press `Alt+Right`. | "Moved, 1 item" |
| 9 | `Tab` to the button "List" of the timeline toolbar, `Enter`. | A dialog "Timeline as a table"; the table is read by row and column ("Name, Type, Track, Start, End, Duration, State"); `Esc` closes it and the focus returns to the button |
| 10 | In the inspector, find the field "Opacity" (tab "Video"), type a value outside the range, `Enter`. | The error is spoken as an alert, and the field is "invalid", with the message as its description |
| 11 | Press `Control+K`. Type "export". | A dialog "Command palette"; the results are read as options; `Esc` returns to where you were |
| 12 | Choose "Export", start it, and leave the focus in the dialog. | The progress said in steps of 10 percent, and "Export finished" at the end |
| 13 | Open "Edit Image" from the navigation, create an image, add a text layer from the toolbar. | The layer appears in the "Layers" tree: "the name, text, ..." with its level and "1 of N" |
| 14 | `Tab` to the canvas, press `PageUp`, then `Shift+Right`. | "Layer the name, text, at x, y, w by h", and a new position after the nudge |
| 15 | Open "Publish". Choose the week view, move to a day with the arrows. | The days are read by their full date; a post is a button with its time, network and state |
| 16 | On a post press `Space`, `Right`, `Enter`. | "Moving to the time" while you move; the new day after `Enter` |
| 17 | Open "Approvals". `Tab` to "Approve" of a request, `Enter`. | The button name includes the summary of the request; after `Enter`: "Approved: the summary", and the focus is on the next request or on the heading "Waiting for you" |
| 18 | Open the chat button at the bottom right. | "Chat, complementary"; the focus is inside the panel; `Esc` closes it and returns to the button |
| 19 | Open "Connections" and set up an account with a wrong value. | The error is an alert and the field is described by it |
| 20 | Turn on "Reduce motion" in Windows (Settings, Accessibility, Visual effects) and repeat step 9. | The dialog opens with no movement |

## What to write down

For each step that fails: the step number, the page address, what NVDA said, what you expected. Put it in `development/feedbacks/vukem-video.md` as a `VV` entry (or ask the orchestrator to) and update the outcome table above. A pass of VoiceOver on macOS follows the same steps and is recorded as a second row of the table when it is done.
