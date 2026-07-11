# localserver-go

Personal Go/Gin automation server integrating Spotify and home device control. Entry point is `main.go` → `server.CreateServer()` (listens on `:9000`).

## Layout

- `server/` — route registration (Gin router groups: `/spotify`, `/manage`, `/corrections`).
- `spotify/` — Spotify Web API integration. `endpoints.go` holds Gin handlers; `spotify.go` holds the `Spotify` env struct + API calls; `utils.go` holds helpers.
- `manage/` — local device control (lamp toggle via ESP32, grammar review).
- `corrections/` — corrections endpoint.

Two Spotify environments exist, `home` and `main` (separate accounts), each with its own devices and tokens under `.tokens/`.

## Testing — REQUIRED on every change

After any code change, run the tests and make sure they pass before considering the work done:

```bash
go build ./...
go test ./...
```

When you add or modify a handler/function, add or update its test in the same package (`*_test.go`). Tests here are pure unit tests — do NOT hit the network or real Spotify. When logic is entangled with live HTTP calls (e.g. token refresh, device fetch), extract the pure part into a small helper and test that (see `filterActiveDevices` in `spotify/endpoints.go` and `TestFilterActiveDevices`). Handler-level tests use `httptest` + `gin.CreateTestContext`; save and restore package globals like `envs` so tests stay isolated.
