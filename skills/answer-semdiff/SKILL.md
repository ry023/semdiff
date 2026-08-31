---
name: answer-semdiff
description: Start a semdiff answer session and answer viewer questions about semantic Groups or Fragments until the viewer ends answer mode. Use when the user asks to answer semdiff or invokes /answer-semdiff.
---

# Answer semdiff questions

Answer questions until the user ends answer mode in the viewer. Do not edit code or `groups.json` unless the user separately asks for a change.

1. Use an explicitly requested groups file when one was supplied. Otherwise omit it from semdiff commands so the CLI resolves `.semdiff/reviews/<base-sha>...<head-sha>/groups.json` from the current grouping draft.
2. Run `semdiff questions session start --json`, adding `<groups-file>` before `--json` only for an explicit file, and retain its `session_id`.
3. Run `semdiff questions wait --session <session-id> --json`, again adding the explicit groups file when applicable. This blocks until the viewer submits a question or ends answer mode.
4. If the event is `stopped`, report that answer mode ended and finish.
5. For a `question` event, treat its `question.history` as the conversational context for `question.question`. Do not infer context from other semdiff threads.
6. Inspect the anchored Group or Fragment. For a Fragment, use `semdiff show <fragment-id> --json`, adding `<groups-file>` before the fragment ID only for an explicit file. Read related fragments, commits, and repository code when needed to explain the intent and relationships accurately. A follow-up may rely on facts or terminology established in `history`.
7. Answer the newest question with concrete references to the change. Clearly distinguish evidence from inference.
8. Write the response to a temporary file and run `semdiff questions answer <question-id> --stdin < <temporary-file>`, adding `<groups-file>` before the question ID only for an explicit file.
9. Return to step 3. Do not finish merely because one answer was registered.
