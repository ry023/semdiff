---
name: answer-semdiff
description: Wait for and answer one question submitted from the semdiff viewer about a semantic Group or Fragment in groups.json. Use when the user asks to answer semdiff or invokes /answer-semdiff.
---

# Answer a semdiff question

Handle one question, then return control to the user. Do not edit code or `groups.json` unless the user separately asks for a change.

1. Identify the requested groups file, using `groups.json` when no path was supplied.
2. Run `semdiff questions wait <groups-file> --json`. This intentionally blocks until the viewer submits a question and exits after claiming one question.
3. Inspect the anchored Group or Fragment in the groups file. For a Fragment, use `semdiff show <groups-file> <fragment-id> --json`. Read related fragments, commits, and repository code when needed to explain the intent and relationships accurately.
4. Answer the actual question with concrete references to the change. Clearly distinguish evidence from inference.
5. Write the response to a temporary file and run `semdiff questions answer <groups-file> <question-id> --stdin < <temporary-file>`.
6. Report that the answer is available in the viewer and finish. Do not wait for another question in the same invocation.
