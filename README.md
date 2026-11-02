# ⚗︎ Experiment Bench

A LAN-hosted Q/A that turns the **experiments, not habits** framework into experiment
cards you can actually run. Built from [Mia's experiment compilation video](https://www.youtube.com/watch?v=Xpp5WT_3uqQ),
which builds on the *experimental mindset* (Big Think / Anne-Laure Le Cunff).

**Core distinction:** a habit is something you *know* is good and commit to indefinitely;
an experiment is something you *don't know* will work — and the point is to find out.
Failure is a data point.

## What it does

- **`/`** — the research summary: mindset grid (cynicism / escapism / perfectionism / experimental
  over ambition × curiosity), the observe → research-question → tiny-experiment → analyze loop,
  and the rules that make runs work (commitment pacts, regularity over streaks, intentions under goals).
- **`/qa`** — a four-stage wizard (Observe → Intentions → Design → Pact) that saves answers
  server-side as you go and compiles a draft experiment card.
- **`/experiments`** — the board: experiment cards with status cycle
  (running → analyzing → kept → revised → dropped) and review dates computed from the pact duration.
- **`/print`** — printable run sheets, one page per experiment, with blank result fields.

## Stack

- Go stdlib (`net/http`, `html/template`, `embed`) — no dependencies
- [htmx v4](https://four.htmx.org/) (vendored) — wizard transitions, board updates
- [missing.css](https://missing.style/) 1.3.0 (vendored) — classless styling + print styles
- State: single JSON file at `data/state.json`
- Dev tooling: [mise](https://mise.jdx.dev/) (Go toolchain, env, tasks)

## Run it

```sh
mise run dev        # build + run in the foreground
mise run test       # smoke-test all pages
mise run install-service   # systemd user unit; survives reboots
```

Env (set in `mise.toml`): `PORT` (8321), `BIND` (0.0.0.0), `DATA_DIR`.

Then open `http://<host-lan-ip>:8321` from any device on the WiFi.

## Resetting

"Start the Q/A over" on `/qa` clears answers but keeps the board. Deleting the whole
`data/` directory resets everything.
