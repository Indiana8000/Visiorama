# Map Feature Plan — Visiorama

## Entscheidungen (aus Grill-Session)

| Thema | Entscheid |
|---|---|
| Datenmenge | < 1.000 Bilder mit GPS |
| Kartenkacheln | Extern erlaubt — **OpenFreeMap.org** (Vector Tiles, kostenlos, kein API-Key) |
| Map-Library | **MapLibre GL** via `maplibre-gl` npm package |
| Clustering | **Server-side, grid-basiert**, on-the-fly (kein Pre-Compute) |
| Cluster-API | `GET /api/map/clusters?zoom=Z&bbox=W,S,E,N&album_id=X` |
| Album-Filter | `album_id` = Album + rekursiv alle Sub-Albums; fehlt = global |
| Einstiegspunkt | Toolbar-Button in AlbumView (nur sichtbar wenn GPS-Bilder vorhanden) |
| Route | `/map?album_id=X` — neue Vue-Route |
| Einzelbild-Marker | Thumbnail direkt im Marker (wie Google Photos) |
| Lightbox | Bestehende LightboxView öffnen bei Klick |
| Performance | On-the-fly, kein Cache nötig — 4 vCPU/4GB weit überdimensioniert |

---

## Architektur-Überblick

```
Browser (Vue 3)                    Go Backend
─────────────────                  ──────────────────────────────
MapView.vue                        GET /api/map/clusters
  └─ MapLibre GL                     └─ media_repo.GetGPSByAlbum(album_id, recursive)
       └─ OpenFreeMap tiles (extern)  └─ gridCluster(points, zoom, bbox)
                                       └─ GeoJSON FeatureCollection zurück

AlbumView.vue
  └─ [Karte] Button (wenn GPS > 0)
       └─ router.push('/map?album_id=X')
```

---

## Backend-Implementierung

### 1. `internal/index/repositories/media_repo.go`
- Neue Methode: `GetGPSMedia(albumID *int64, recursive bool) ([]GPSMedia, error)`
  - Query: `SELECT id, gps_lat, gps_lon FROM media WHERE gps_lat IS NOT NULL AND gps_lon IS NOT NULL`
  - Mit `album_id`: rekursiv alle Sub-Album-IDs sammeln via CTE, dann filtern
  - Ohne `album_id`: alle Bilder global

### 2. `internal/map/cluster.go` (neue Datei)
- Grid-Clustering Algorithmus:
  ```go
  type ClusterPoint struct { Lat, Lon float64; IDs []int64 }
  
  func GridCluster(points []GPSMedia, zoom int, bbox BBox) []ClusterPoint
  ```
  - Zoom → Gitterzellgröße (z.B. Zoom 3 = 10°-Zellen, Zoom 10 = 0.01°-Zellen)
  - Formel: `cellSize = 360 / (256 * 2^zoom / tileSize)`
  - Punkte außerhalb BBox verwerfen
  - Punkte in gleicher Zelle aggregieren → Centroid + ID-Liste

### 3. `internal/api/handlers_map.go` (neue Datei)
- `GET /api/map/clusters?zoom=&bbox=&album_id=`
  - Parameter parsen + validieren
  - `GetGPSMedia` aufrufen
  - `GridCluster` aufrufen
  - GeoJSON FeatureCollection zurückgeben:
    ```json
    {
      "type": "FeatureCollection",
      "features": [
        {
          "type": "Feature",
          "geometry": { "type": "Point", "coordinates": [lon, lat] },
          "properties": {
            "count": 5,
            "ids": [1, 2, 3, 4, 5],
            "thumbnail_id": 1
          }
        }
      ]
    }
    ```

### 4. Router registrieren
- In `internal/api/` Router-Setup: `GET /api/map/clusters` → handler

---

## Frontend-Implementierung

### 1. `npm install maplibre-gl` in `/web/app/`

### 2. `web/app/src/views/MapView.vue` (neue Datei)
- MapLibre GL Map initialisieren mit OpenFreeMap style URL
- On mount: `/api/map/clusters?zoom=&bbox=&album_id=` fetchen
- Map `moveend`/`zoomend` → neu fetchen mit aktueller BBox + Zoom
- GeoJSON-Layer für Cluster-Punkte
- Custom Marker:
  - Count > 1: Kreis mit Zahl
  - Count = 1: Thumbnail (`/api/media/{id}/thumbnail`) als Marker-Bild
- Klick auf Cluster: `map.flyTo()` + Zoom+1
- Klick auf Einzelbild: `router.push('/media/{id}')`

### 3. `web/app/src/router/index.js`
- Route hinzufügen: `{ path: '/map', component: MapView, props: route => ({ albumId: route.query.album_id }) }`

### 4. `web/app/src/views/AlbumView.vue`
- Neue Computed: `hasGPSMedia` — API-Aufruf `GET /api/albums/{id}/gps-count` oder aus bestehendem Album-Metadaten
- Button in Toolbar: `<button v-if="hasGPSMedia" @click="openMap">Karte</button>`
- `openMap()`: `router.push('/map?album_id=' + album.id)`

### 5. `web/app/src/api/client.js`
- `fetchMapClusters(zoom, bbox, albumId)` hinzufügen

---

## Neuer API-Endpoint für GPS-Count (optional, für Button-Visibility)

`GET /api/albums/{id}/gps-count` → `{ "count": 42 }`

Alternativ: GPS-Count in bestehendem Album-Response mitliefern (sauberer).

---

## Zoom → Gitterzellgröße Mapping

| Zoom | Zellgröße | Typische Ansicht |
|------|-----------|-----------------|
| 0-2  | 20°       | Kontinente |
| 3-5  | 5°        | Länder |
| 6-8  | 1°        | Regionen |
| 9-11 | 0.1°      | Städte |
| 12-14| 0.01°     | Stadtteile |
| 15+  | 0°        | Kein Clustering |

---

## Implementierungs-Reihenfolge

- [ ] 1. Backend: `GetGPSMedia` in media_repo
- [ ] 2. Backend: `cluster.go` Grid-Algorithmus
- [ ] 3. Backend: `handlers_map.go` + Router
- [ ] 4. Backend: GPS-Count in Album-Response
- [ ] 5. Frontend: `npm install maplibre-gl`
- [ ] 6. Frontend: `MapView.vue`
- [ ] 7. Frontend: Router-Route
- [ ] 8. Frontend: AlbumView Toolbar-Button
- [ ] 9. Frontend: `client.js` API-Methode
- [ ] 10. Test: End-to-end mit echten GPS-Bildern

---

## Offene Fragen / Risiken

- OpenFreeMap Style-URL muss geprüft werden (aktuell: `https://tiles.openfreemap.org/styles/liberty`)
- MapLibre GL Lizenz: BSD 3-Clause ✓
- Thumbnail-Marker: CORS-Probleme wenn Backend + Frontend auf gleichem Port → kein Problem (same-origin)
- Rekursive Album-CTE: SQLite unterstützt `WITH RECURSIVE` seit 3.8.3 ✓
