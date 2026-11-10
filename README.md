# TotalBotWar

An original turn-based strategy MCP game inspired by large army tactics. Command five regiments of the Dawn Legion against the Obsidian Host across the partially observable battlefield of Ashen Vale. Open the game fullscreen from ChatGPT’s sidebar, then give orders in the ChatGPT composer or directly on the map.

This first version is a complete single-player skirmish, with a deterministic tactical opponent. It has no external model calls, API key requirement, multiplayer, or campaign economy.

## Run

Requires Go 1.26+. Node.js 22+ is required to change or rebuild the UI. The generated UI is committed with the source and embedded in the Go executable.

```sh
go run ./cmd/server
```

The Streamable HTTP MCP endpoint is `http://127.0.0.1:3005/mcp`; `/health` returns server health. The default data directory is `./data`.

```sh
npm ci
npm run build
./build/TotalBotWar

# Configuration
HOST=0.0.0.0 PORT=8000 DATA_DIR=/persistent/battles go run ./cmd/server
./build/TotalBotWar --address 127.0.0.1:3005 --endpoint /mcp
./build/TotalBotWar --stdio
./build/TotalBotWar --version
```

## Connect in ChatGPT

1. Run or deploy the server. A remote ChatGPT client needs a reachable **HTTPS** MCP URL. A public HTTPS tunnel to port 3005 is sufficient for development; a production host should mount persistent storage for `DATA_DIR`.
2. Add that URL as an MCP connection in ChatGPT’s developer/plugin settings, following the [official connection guide](https://developers.openai.com/plugins/deploy/connect-chatgpt). Enable the connection for the conversation.
3. Open **TotalBotWar** from the sidebar, then choose **Begin the campaign**. Alternatively ask “Start a TotalBotWar battle and open the battlefield.” The model calls `create_battle`, then `open_battlefield` with the returned ID.
4. Use ChatGPT’s composer, the game’s **Send to chat** field, or the map to command your army. The in-game field routes through `ui/message`; in a global app it can start a conversation, while an active chat receives follow-up commands. It is enabled only when the host advertises this extension.
5. Save the battle ID shown below the map. It can resume the game in another view or conversation with `get_battle` or `open_battlefield`. The lobby also accepts the ID.

The server declares both `global` and `thread` entrypoints on `open_battlefield`, and fullscreen display metadata on the UI resource and during MCP Apps initialization. Sidebar entries and chat extensions depend on host support and account availability. Ordinary MCP clients can play entirely through the tools.

A local plugin manifest is included in `.codex-plugin/plugin.json`, with a stdio connection in `.mcp.json`. Build the executable before using this package. Creating the project does not install the plugin into your ChatGPT account or publish a remote endpoint.

References: [Sidebar extensions](https://developers.openai.com/plugins/build/extensions#sidebar-apps), [MCP UI and state](https://developers.openai.com/plugins/build/chatgpt-ui), [Fullscreen resource metadata](https://developers.openai.com/plugins/reference#declare-display-modes-before-the-ui-loads).

## Give orders in chat

Try:

- “Read my battle and suggest a plan using only what my scouts can see.”
- “Move Grey Watch toward the watchtower at I6. Hold the rest of the army.”
- “Have Ember Bows attack the visible enemy within range.”
- “Queue those orders and resolve the turn.”

The model translates natural language to structured orders. The Go engine validates and executes them. Chat tools and map buttons share the same authoritative state. Data tools do not remount the UI; the mounted app polls `get_battle` every 2.5 seconds while visible and updates model context with the battle ID, revision, selected regiment and orders. It stops polling during UI mutations, when hidden, and on teardown.

| Tool | Purpose |
| --- | --- |
| `open_battlefield(battle_id?)` | Open the sidebar/fullscreen UI; omitted ID shows the lobby. |
| `create_battle()` | Start a separate saved battle and return the player-visible state. |
| `get_battle(battle_id)` | Read current visible state, queued orders, turn and revision. |
| `issue_orders(battle_id, revision, orders)` | Atomically queue/replace 1–5 orders; never advances time. |
| `end_turn(battle_id, revision)` | Resolve both armies and advance to planning or a terminal result. |

Example order batch (replace the battle ID and revision with the latest returned values):

```json
{
  "battle_id": "<48-character ID from create_battle>",
  "revision": 1,
  "orders": [
    {"unit_id":"p1","action":"move","x":3,"y":2},
    {"unit_id":"p3","action":"defend"},
    {"unit_id":"p5","action":"scout"}
  ]
}
```

Call `end_turn` with the **new** revision returned by `issue_orders`. Revisions change on every successful mutation, including order edits. Concurrent/stale writes fail instead of applying twice. On a stale error, read the latest battle and reconsider the command before retrying.

## Rules

- The map is 18 × 12. Coordinates are zero-based in tools; A1 is `(0,0)`, I6 is `(8,5)`.
- Each unit receives one order per turn: `move`, `attack`, `defend`, or `scout`. `cancel` removes its queued order. Unordered units defend.
- Both sides plan against limited information. Both move before attacks; attacks deal damage simultaneously. Targets may move out of range or sight. Movement initiative alternates by turn. An occupied route stops movement; units never stack.
- Infantry, spears and archers move 2 orthogonal tiles; cavalry 3; scouts 4. Water is impassable. Terrain is charted and publicly known. The highlighted movement area shows terrain reach, and cannot reveal hidden occupants.
- Archers attack within 3 orthogonal tiles; other units attack adjacent tiles. Attacks require current visibility and line of sight. Spears deal bonus damage to cavalry.
- Base vision is 4 tiles, or 6 for scouts. Intermediate forests block sight; units inside a forest are concealed beyond 2 tiles. Hills and the tower add 1 vision. A scout order adds 2 vision during resolution and the next planning phase.
- Defend reduces damage by one third and restores 8 morale. Forests, hills and the tower reduce damage by one quarter. At 0 health or 20 morale a regiment breaks and leaves the battle. No reinforcements or healing.
- Occupying I6 scores 1 point at the end of the turn. Score 6 points or eliminate the enemy to win. After 24 turns, higher tower score wins; equal scores draw. Scores and tower ownership reports are public strategic information.
- The bot sees only its own army and currently visible opponents. It pursues the tower or engages visible enemies. The model and UI receive friendly units and **only currently visible** enemies, with no enemy order list or hidden-unit metadata. Dispatches redact unseen attacker identities.

## Storage and access

Battles are JSON files written via temporary file, sync and atomic rename. Writes are serialized under a process mutex, with revision checks. Use one server process per data directory; multi-replica hosting requires a transactional shared database.

This version uses unguessable, random 192-bit battle IDs as bearer capabilities. Anyone with a battle ID can read and command that battle. There is no account binding, public battle listing, OAuth or matchmaking. Treat IDs as private. Bind to loopback by default; add account authorization and request limits before offering a public multi-user service. The UI caches only the last battle ID as a convenience; game state always lives on the server. Back up and persist the data directory across container replacements.

## Verify

```sh
npm run build
npm test                     # Go tests with race detector
go vet ./...

# Browser integration test against the real HTTP MCP transport
BROWSER_EXECUTABLE='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' npm run test:browser
# Or install the Playwright browser first:
npx playwright install chromium
npm run test:browser

# Development host emulator; captures chat messages without running a chat model
node scripts/browser-smoke.mjs --preview
```

The browser test verifies sidebar registration, fullscreen initialization, a full SDK handshake, player-visible rendering, movement previews and orders, turn resolution, chat-side polling, stale-write rejection, invalid inputs, server failure/retry, message routing, resume, compact health bars and mobile overflow. The Go suite covers fog, forest concealment, transactional order batches, simultaneous combat, victory, turn limits, persistence, concurrent requests and completed matches. Real ChatGPT account installation remains a separate end-to-end check.

## Docker

```sh
docker build -t totalbotwar .
docker run --rm -p 3005:3005 -v totalbotwar-data:/data totalbotwar
```

The image builds and embeds the UI, then runs a Go binary as a non-root user. The volume preserves battles.

## Source map

- `internal/battle/engine.go`: simulation, bot decisions and redacted player views.
- `internal/battle/store.go`: persistent battle transactions.
- `internal/battle/server.go`: tool schemas, sidebar registration and UI resource.
- `web/battlefield.ts`: official MCP Apps/extension bridge, map interaction and synchronization.
- `web/shell.html`: responsive battlefield, lobby, command deck and field manual.
- `scripts/build-ui.mjs`: bundles the SDK and app into the embedded `internal/battle/battlefield.html`.
- `scripts/browser-smoke.mjs`: real HTTP MCP + mock ChatGPT host integration test and local preview.
- `cmd/server/main.go`: HTTP/stdio transport, flags, health and graceful shutdown.

The game uses original names, code and vector/CSS visuals. It does not include Total War assets or branding.
