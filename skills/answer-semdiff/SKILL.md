---
name: answer-semdiff
description: Wait for and answer one question submitted from the semdiff viewer about a semantic Group or Fragment in groups.json. Use when the user asks to answer semdiff or invokes /answer-semdiff.
---

# Answer a semdiff question

Handle one question, then return control to the user. Do not edit code or `groups.json` unless the user separately asks for a change.

1. Use an explicitly requested groups file when one was supplied. Otherwise omit it from semdiff commands so the CLI resolves `.semdiff/reviews/<base-sha>...<head-sha>/groups.json` from the current grouping draft.
2. Run `semdiff questions wait --json`, adding `<groups-file>` before `--json` only for an explicit file. This intentionally blocks until the viewer submits a question and exits after claiming one question. Its `history` contains only earlier user and assistant messages from the same thread; treat that sequence as the conversational context for the new `question`. Do not infer context from other semdiff threads.
3. Inspect the anchored Group or Fragment. For a Fragment, use `semdiff show <fragment-id> --json`, adding `<groups-file>` before the fragment ID only for an explicit file. Read related fragments, commits, and repository code when needed to explain the intent and relationships accurately. A follow-up may rely on facts or terminology established in `history`.
4. Answer the newest `question` in the context of `history`, with concrete references to the change. Clearly distinguish evidence from inference.
5. Write the response to a temporary file and run `semdiff questions answer <question-id> --stdin < <temporary-file>`, adding `<groups-file>` before the question ID only for an explicit file.
6. Report that the answer is available in the viewer and finish. Do not wait for another question in the same invocation.
