# Quality Review Checklist

Generated: 2026-07-17  
Scope: Bug-Fixes → Security-Hardening → Code-Cleanup  
Context: LAN-only deployment, read-only access, no auth layer

---

## Phase 1: Bug-Fixes 🔴

- [x] **B1** `internal/index/migrations.go:94` — Wrap DROP/INSERT/RENAME in explicit transaction → prevents data loss on power failure
- [x] **B2** `web/app/src/views/LightboxView.vue:182` — Add NaN-guard after `parseInt(route.params.id, 10)` → prevents silent API errors
- [x] **B3** `web/app/src/api/client.js:39` — *(no fix needed — URLSearchParams already used correctly)*

---

## Phase 2: Security-Hardening 🟡

- [x] **S1** `internal/api/handlers_admin.go:20` — Validate CacheDir is under safe parent before `os.RemoveAll` → prevents catastrophic deletion on misconfiguration
- [x] **S2** `internal/api/handlers_map.go:59,109` — Sanitize Host header via `safeHost()` + fix nil-panic on `http.NewRequestWithContext` error
- [x] **S3** `internal/api/handlers_transcode.go:74` — Validate OutputPath against Transcode.CacheDir before `os.Open`
- [x] **S4** `internal/scan/runner.go:34` — Mutex in `SetWarmer()` + capture warmer locally before goroutine launch

---

## Phase 3: Code-Cleanup 🔵

- [x] **C1** `internal/api/handlers_media.go:107` — Remove duplicate `mediaRepo2`, reuse `mediaRepo`
- [x] **C2** `internal/scan/runner.go:58,105` — Replace silent `_ =` with `slog.Warn` for DB status updates
- [x] **C3** `internal/transcode/runner.go:95-144` — Log silently-ignored status update errors
- [x] **C4** `web/app/src/views/LightboxView.vue:376` — Add 150-poll max-retry counter to `pollTranscode`
- [x] **C5** `internal/convert/cache.go:38` — *(no fix needed — expiry check is correctly inside mutex)*

---

## Status

- Total: 12 items
- Done: 10 (B1, B2, S1-S4, C1-C4)
- Not needed: 2 (B3, C5 — findings were false positives)
- Build: ✅ `go build ./...` passes
