# ADR-0051: Agents never start a capture

- Status: Accepted (2026-11-23, author decision D-133)
- Deciders: author
- Links: ADR-0032, ADR-0033, RF-135, RF-136, SDD-007, SDD-015 section 4, epic 57, epic 64

This record decides what an agent may and may not do with screen, camera and microphone capture.

## 1. Context

Capture can expose passwords, messages, documents and the people in a room. Stuarty treats agents as first-class users and also treats them as hijackable: a prompt injection in a file or page must not reach the user's screen. The product already enforces that only a person in a privileged browser session decides an approval (`service/approval/approval.go`, `CheckPrivileged`).

## 2. Decision

1. **Only a person in a privileged browser session starts or resumes a capture.** The rule lives in the service, under every adapter, and returns `E_PRIVILEGED_SESSION` to anyone else.
2. **No approval rule can authorize a capture.** Unlike a publication, it has no standing or automatic approval.
3. An agent may prepare a recording, read its state, mark takes and propose cuts. It may stop or pause a capture, since that only reduces exposure.
4. **The indicator is always visible** from the helper's start until its exit, driven by the host and not by the page. On the web, the browser's indicator and a banner fill this role.
5. Keystrokes are never recorded. Cursor and click events stay local and are deleted with the recording.
6. A recording is not sent anywhere by capture. Publication keeps its own approval.
7. A finished recording enters the project only when a person confirms it, on a step that states that agents with access to the project can read it. After that, an agent reads it as it reads other project media.

## 3. Alternatives

| Option | Why not |
| --- | --- |
| An agent starts capture after one approval | Turns a click into standing access to the screen |
| An approval rule for trusted agents | A hijacked trusted agent records the user |
| Allow capture of one chosen window by an agent | The window's content can change, and the risk stays |

## 4. Consequences

- Fully unattended recording does not exist by design.
- Epic 61 depends on epic 57, because a password session is what identifies a person in local mode. The `stuarty open` link stays unprivileged.
- Each surface gets a security test that proves no helper process starts for an agent.

## 5. Decision of the author (D-133)

Item 7 stands as written: the confirmation at the end of a recording is the grant, with no separate permission per recording.
