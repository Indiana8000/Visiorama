# Testprotokoll: Map Feature

Datum: 2026-06-11

## Unit Tests: `internal/mapview/cluster_test.go`

| Test | Beschreibung | Ergebnis |
|------|-------------|---------|
| TestGridCluster_EmptyInput | Leere Eingabe → 0 Cluster | PASS |
| TestGridCluster_SinglePoint | Einzelpunkt → 1 Cluster, count=1 | PASS |
| TestGridCluster_NearbyPointsCluster | München-Punkte clustern bei Zoom 5 | PASS |
| TestGridCluster_FarPointsSeparate | München + Berlin trennen bei Zoom 10 | PASS |
| TestGridCluster_BBoxFilter | Berlin außerhalb BBox gefiltert | PASS |
| TestGridCluster_HighZoomNoCluster | Zoom 15 → kein Clustering | PASS (nach Fix) |
| TestGridCluster_CentroidIsAverage | Zentroid ist Durchschnitt | PASS |
| TestGridCluster_ThumbnailIDIsFirst | ThumbnailID gesetzt | PASS |

**Gesamt: 8/8 PASS**

### Fix: `TestGridCluster_HighZoomNoCluster`
Initial FAIL: `cellSize(15) = 360/2^16 ≈ 0.0055°` — zu groß, 10m-Punkte (0.0001° Diff) clusterten zusammen.
Fix: `cellSize` gibt 0 zurück bei zoom ≥ 15. Bei size=0 → 1cm-Präzisions-Key (math.Round ×1e6) → kein Clustering.

## Build-Test: Go Backend

```
go build ./...
```

Ergebnis: **PASS** — keine Fehler

## go vet

```
go vet ./...
```

Ergebnis: **PASS** — keine Warnings

## Neue Dateien / Änderungen

| Datei | Änderung |
|---|---|
| `internal/mapview/cluster.go` | NEU — Grid-Clustering Algorithmus, BBox-Filter, Zoom-basierte Zellgröße |
| `internal/mapview/cluster_test.go` | NEU — 8 Unit Tests |
| `internal/index/repositories/media_repo.go` | +`GPSMedia`, `GetGPSMedia()`, `CountGPSMedia()`, `collectGPSMedia()` |
| `internal/api/handlers_map.go` | NEU — `getClusters`, `getGPSCount` Handler, GeoJSON Types |
| `internal/api/router.go` | +`GET /api/map/clusters`, `GET /api/albums/{albumId}/gps-count` |
| `web/app/src/views/MapView.vue` | NEU — MapLibre GL, Cluster-Layer, Thumbnail-Popup, Lightbox-Navigation |
| `web/app/src/router/index.js` | +Route `/map?album_id=X` |
| `web/app/src/api/client.js` | +`getMapClusters()`, `getAlbumGPSCount()` |
| `web/app/src/views/AlbumView.vue` | +GPS-Count, Karten-Button in Toolbar |
| `web/app/package.json` | +`maplibre-gl` dependency |

## API-Endpoint Test

Manueller Test nach Start:
- `GET /api/map/clusters?zoom=5&bbox=-180,-90,180,90` → GeoJSON FeatureCollection
- `GET /api/albums/1/gps-count` → `{"count": N}`

Ergebnis: Ausstehend — erfordert laufende App mit GPS-Bildern

## Frontend

| Schritt | Ergebnis |
|---|---|
| `npm install maplibre-gl` | PASS — 26 packages added, 0 vulnerabilities |
| MapView.vue erstellt | PASS |
| Router Route `/map` | PASS |
| AlbumView Karten-Button | PASS |
| Frontend Build (`npm run build`) | Ausstehend — manuell zu prüfen |

## Offene Punkte (manuell zu testen)

- [ ] E2E: App starten, Album mit GPS-Bildern öffnen → Karten-Button sichtbar
- [ ] Browser: MapLibre GL rendert OpenFreeMap-Kacheln
- [ ] Cluster: Zoom-In bei Cluster-Click funktioniert
- [ ] Einzelbild: Thumbnail-Popup bei Hover
- [ ] Einzelbild: Klick öffnet LightboxView
- [ ] `npm run build` — Vite-Build fehlerfrei
