# semantic-diff

`semantic-diff` reorganizes a Git range into semantic review groups without changing Git history.

```sh
go build -o semantic-diff ./cmd/semantic-diff
./semantic-diff commits main..HEAD --json
./semantic-diff fragments main..HEAD --json
./semantic-diff show F-0123456789ab --json
./semantic-diff validate groups.json
./semantic-diff view groups.json --addr 127.0.0.1:8080
```

`fragments` caches the latest complete inventory under `.semantic-diff/`; its JSON output omits patches so an agent can inspect individual changes with `show`. A `groups.json` records full resolved commit SHAs and version `1`. See [the grouping skill](skills/semantic-grouping/SKILL.md) for the staged agent workflow.
