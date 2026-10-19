# Contributing

This is a hobby project I started to keep track of the AI-model tests I run for other work, and to get a clear view of model costs and capabilities. I have limited time for it right now, but contributions are very welcome. 

## How to contribute

1. Fork the repository and create a branch for your change.
2. Make your change, keeping it focused and matching the existing style. [architecture.md](architecture.md) says where each kind of code goes and which rules the data model has to keep.
3. Make sure it still builds and tests pass: `go build ./...` and `go test ./...`.
4. If you changed anything in the page, check it in a browser yourself. The Go tests cover the server; there is no JavaScript test runner here, so nothing checks `internal/app/index.html` or `internal/app/static/` automatically. Say in the pull request what you clicked through.
5. Open a pull request that explains what you changed and why.

## Review

Pull requests are reviewed before merging, with AI assistance to help check correctness and quality. Reviews and replies can be slow, so please be patient and open to feedback. If you do not hear back for a while, it is lack of time, not lack of interest.

## Guidelines

- Keep changes focused; smaller pull requests are easier to review and merge.
- The documents under `docs/` are overviews, not references: the code is the truth, and `GET /api` on a running server is the API reference. Change a document only where a change made its overview wrong.
- By contributing, you agree that your contributions are licensed under the project's MIT license.
