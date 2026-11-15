# Featureban Portfolio Demo Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development. Complete the independent file scopes below, then integrate and review the whole branch.

**Goal:** A reliable local multiplayer demo and useful game analytics for a personal portfolio.
**Architecture:** Keep the existing Go/React application, one Go server and SQLite snapshots. HTTP mutations commit before becoming visible; WebSocket and HTTP share revisioned public state.
**Tech Stack:** Go 1.26, modernc.org/sqlite, React/TypeScript/Vite, pytest, Docker Compose.
**Spec:** `docs/superpowers/specs/2026-11-05-portfolio-demo-design.md`

## Global Constraints
- Preserve upstream history, team attribution and LICENSE; work on feature/portfolio-demo.
- One writer per file; root owns integration, commits, push and final verification.
- Keep existing game rules, including WIP and max_days behavior.
- No live VPS access or public deployment; no credentials in tracked working files.
- Write focused failing checks before nontrivial implementation.

## Review Focus
- Duplicate nickname must never recover someone else's session.
- SQLite failure must leave HTTP, WebSocket and visible game state unchanged.
- Restart must preserve private turn progress, sessions and unique IDs.
- Retro/finish must not duplicate daily snapshots or mislabel partial periods.
- A delayed response or old socket event must not replace a newer revision.

## Tasks
- [x] Backend core: own `app/backend/main.go`, persistence/session implementation and core Go tests, go.mod/go.sum. Add per-room cookie sessions (7 days), `/me`, SQLite snapshots and mutation atomicity. Integrate report hooks/routes. Verify auth, restart/failure and concurrent requests with `go test -race ./...`.
- [x] Reporting: own `app/backend/reports.go` and report Go tests. Define `DaySnapshot`, `GameReport`, `captureDay(g, endIteration, partial)` and `buildReport(g)`. Backend core adds `Game.DailySnapshots []DaySnapshot`. JSON contains code/revision/phase/finished/summary/days/iterations; daily entries contain day/iteration/partial/metrics, iteration entries number/start_day/end_day/partial/throughput/average_wip/lead_time/cycle_time. Export CSV with daily metrics. Test a fixed scenario and boundaries.
- [x] Frontend: own `app/frontend/**` except Dockerfile. Restore missing helpers, session-based identity, reconnect and revision filtering, report/demo routes, keyboard actions and client typecheck. Add focused client tests and browser lifecycle coverage. Consume report contract above.
- [x] Test adaptation: own `tests/**`. Record baseline failures; preserve existing HTTP checks while using real per-player session cookies. Update obsolete rule expectations to documented existing behavior; add auth/report/restart coverage without bypassing security.
- [x] Delivery: root owns Compose/nginx/Docker/CI/docs. Provide one-command local HTTP startup and persistent volume, sanitized deploy template, current rules/metrics documentation and screenshots. Run all checks, recreate containers to verify durability, review the integrated diff, commit and push feature branch to the personal fork.
