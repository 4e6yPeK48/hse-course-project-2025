# Featureban portfolio demo

The approved first release keeps the original Go/React game and team attribution,
adds authenticated participants, durable single-server games, reconnecting clients,
and a facilitator report with daily flow metrics, iteration comparison and CSV.

Creation and joining establish persistent per-room HttpOnly sessions. Public player
IDs never authorize actions. Duplicate nicknames return 409. A `/me` endpoint restores
the browser's own identity. SQLite stores complete private game snapshots and session
hashes in a persistent volume. Failed writes neither change published state nor
acknowledge actions. Persisted revisions prevent stale HTTP/WebSocket responses.

Reports are available at `/game/:code/report` and the facilitator-only API
`GET /api/game/{code}/report`, with `?format=csv` for download. Daily snapshots retain
real game/retro boundaries; the final partial interval is labelled. A public bundled
example is explicitly marked as a demo. Main game actions support buttons/keyboard.

One-command local HTTP startup, updated instructions/screenshots and CI cover Go
tests/race detection, existing pytest scenarios, client type checking/build and a
browser lifecycle test. The original team history and LICENSE remain intact.

The first release uses one backend process and small game groups. It does not change
WIP/max_days rules or deploy public hosting. Existing VPS credentials are removed from
the current tree without rewriting upstream history; their owner must rotate them
if still active.
