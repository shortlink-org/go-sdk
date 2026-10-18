# Working in go-sdk

Rules for coding agents and humans alike. The repo is a Go workspace of
independent modules (one `go.mod` per top-level directory, listed in `go.work`).

## Tests: one module at a time

- **Never run tests for the whole workspace in one go** (`go test ./...` from the
  root, or a loop over every module). `db`, `outbox` and `s3` start Docker
  containers via testcontainers, and 32 modules back to back heat a laptop to
  the point of being unusable.
- Run tests only for the module you changed:

  ```bash
  make test MODULE=cqrs
  ```

- Build tags default to `unit`. Container-backed tests need extra tags and
  must be requested explicitly by the user:

  | Module   | Tags            |
  |----------|-----------------|
  | db       | `unit,database` |
  | outbox   | `unit,database` |
  | s3       | `unit,media`    |

  ```bash
  make test MODULE=db TAGS=unit,database
  ```

- `make test` runs with `-race` and `-p 1` by default. Keep it: one module at a
  time is affordable, and the race detector is the only way to find data races.
  `RACE=0` is for quick iteration only, never for the final check.
- `make test-all` exists for a deliberate full pass. It is sequential and
  CPU-capped, still slow, and never the default.

## Builds and modules

- Targets run with `GOWORK=off`, the same as CI: every module must resolve
  through its own `go.mod` and `go.sum`. If a build says
  `updates to go.mod needed`, run `make tidy MODULE=<name>` and commit the result.
- A bare `go build` inside a module in workspace mode can pick up the root
  `vendor` dir. Use `go build -mod=readonly ./...` or the Makefile targets.
- Lint one module: `make lint MODULE=<name>` (golangci-lint, latest, never pinned).

## Repo conventions

- This is a public SDK. Code unused inside this repo is not unused; downstream
  consumers are invisible here. Do not remove exported API on that basis.
- Libraries do not panic. Return typed errors and let the caller decide.
- Testcontainer images stay on `:latest`. Fix the root cause when a container
  test breaks instead of pinning a tag.
- GitHub Actions in `.github/workflows` are pinned by commit SHA with a `# vN`
  comment.
- Do not commit tool output (lint logs, coverage files, compiled binaries).
