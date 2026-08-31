---
name: answer-semdiff
description: Start a semdiff answer session and answer viewer questions about semantic Groups or Fragments until the viewer ends answer mode. Use when the user asks to answer semdiff or invokes /answer-semdiff.
---

# Answer semdiff questions

Answer questions until the user ends answer mode in the viewer. Do not edit code or `groups.json` unless the user separately asks for a change.

1. Use an explicitly requested groups file when one was supplied. Otherwise run `semdiff reviews resolve --json` and use its `groups_path` explicitly for every following command. If `found` is false, report that there is no compatible local review to answer; do not rely on a grouping draft. This matches `semdiff view` when it falls back to an ancestor review.
2. Run `semdiff questions session start <groups-file> --json` and retain its `session_id`.
3. Run `semdiff questions wait <groups-file> --session <session-id> --json`. This blocks until the viewer submits a question or ends answer mode.
4. If the event is `stopped`, report that answer mode ended and finish.
5. For a `question` event, treat its `question.history` as the conversational context for `question.question`. Do not infer context from other semdiff threads.
6. Inspect the anchored Group or Fragment. For a Fragment, use `semdiff show <groups-file> <fragment-id> --json`. Read related fragments, commits, and repository code when needed to explain the intent and relationships accurately. A follow-up may rely on facts or terminology established in `history`.
7. Answer the newest question with concrete references to the change. Clearly distinguish evidence from inference.
8. Write the response to a temporary file and run `semdiff questions answer <groups-file> <question-id> --stdin < <temporary-file>`.
9. Return to step 3. Do not finish merely because one answer was registered.
