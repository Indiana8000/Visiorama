<template>
  <div class="map-view">
    <div class="map-toolbar">
      <button class="btn-back" @click="goBack">← Back</button>
      <span class="map-title">{{ title }}</span>

    </div>
    <div ref="mapContainer" class="map-container" />
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { api } from '../api/client.js'

const props = defineProps({
  albumId: { type: String, default: null },
})

const route = useRoute()
const router = useRouter()
const mapContainer = ref(null)
let map = null

const title = computed(() => props.albumId ? 'Album on Map' : 'All Photos on Map')

function goBack() {
  if (props.albumId) {
    router.push(`/album/${props.albumId}`)
  } else {
    router.push('/')
  }
}

function getBBox(mapInstance) {
  const bounds = mapInstance.getBounds()
  return {
    west: bounds.getWest(),
    south: bounds.getSouth(),
    east: bounds.getEast(),
    north: bounds.getNorth(),
  }
}

async function loadClusters(mapInstance) {
  const zoom = Math.floor(mapInstance.getZoom())
  const bbox = getBBox(mapInstance)
  const albumId = props.albumId ? parseInt(props.albumId, 10) : null

  try {
    const data = await api.getMapClusters(zoom, bbox, albumId)
    const source = mapInstance.getSource('clusters')
    if (source) {
      source.setData(data)
    } else {
      mapInstance.addSource('clusters', { type: 'geojson', data })

      // Cluster circle layer
      mapInstance.addLayer({
        id: 'cluster-circle',
        type: 'circle',
        source: 'clusters',
        filter: ['>', ['get', 'count'], 1],
        paint: {
          'circle-color': '#3b82f6',
          'circle-radius': [
            'interpolate', ['linear'], ['get', 'count'],
            1, 14,
            10, 20,
            100, 30,
          ],
          'circle-opacity': 0.85,
        },
      })

      // Cluster count label
      mapInstance.addLayer({
        id: 'cluster-count',
        type: 'symbol',
        source: 'clusters',
        filter: ['>', ['get', 'count'], 1],
        layout: {
          'text-field': ['to-string', ['get', 'count']],
          'text-font': ['Open Sans Bold'],
          'text-size': 12,
        },
        paint: {
          'text-color': '#ffffff',
        },
      })

      // Single image marker layer (count === 1)
      mapInstance.addLayer({
        id: 'single-marker',
        type: 'circle',
        source: 'clusters',
        filter: ['==', ['get', 'count'], 1],
        paint: {
          'circle-color': '#10b981',
          'circle-radius': 10,
          'circle-stroke-color': '#fff',
          'circle-stroke-width': 2,
        },
      })
    }
  } catch (e) {
    console.error('loadClusters error', e)
  }
}

let popup = null

function mapStateQuery(mapInstance) {
  const center = mapInstance.getCenter()
  const q = new URLSearchParams({
    from: 'map',
    lat: center.lat.toFixed(6),
    lng: center.lng.toFixed(6),
    zoom: mapInstance.getZoom().toFixed(2),
  })
  if (props.albumId) q.set('album_id', props.albumId)
  return q
}

function getBestAnchor(mapInstance, point) {
  const canvas = mapInstance.getCanvas()
  const w = canvas.clientWidth
  const h = canvas.clientHeight
  const vertical = point.y > h / 2 ? 'bottom' : 'top'
  const horizontal = point.x < w * 0.3 ? '-left' : point.x > w * 0.7 ? '-right' : ''
  return vertical + horizontal
}

function buildThumbsHTML(allIds, BASE) {
  const MAX = 12
  const shown = allIds.slice(0, MAX)
  const imgs = shown.map(id =>
    `<img data-id="${id}" src="${BASE}/api/media/${id}/thumbnail?size=320" class="thumb-img" />`
  ).join('')
  const more = allIds.length > MAX
    ? `<div class="thumb-more-badge" data-action="albums">+${allIds.length - MAX} weitere in Alben</div>`
    : ''
  return `<div class="thumb-grid">${imgs}</div>${more}`
}

function buildAlbumsHTML(albums, BASE) {
  if (!albums || albums.length === 0) {
    return `<div class="thumb-albums-empty">Keine Alben gefunden</div>`
  }
  const rows = albums.map(a => {
    const cover = a.coverThumbnailUrl
      ? `<img src="${BASE}${a.coverThumbnailUrl}" class="thumb-album-cover" />`
      : `<div class="thumb-album-cover thumb-album-cover--empty"></div>`
    return `<div class="thumb-album-row" data-album-id="${a.id}">
      ${cover}
      <div class="thumb-album-info">
        <div class="thumb-album-name">${a.name}</div>
        <div class="thumb-album-count">${a.matchCount} Foto${a.matchCount !== 1 ? 's' : ''}</div>
      </div>
    </div>`
  }).join('')
  return `<div class="thumb-albums-list">${rows}</div>`
}

async function showThumbnailPopup(mapInstance, lngLat, point) {
  if (popup) { popup.remove(); popup = null }

  // queryRenderedFeatures collects ALL stacked markers at the click pixel
  const features = mapInstance.queryRenderedFeatures(point, { layers: ['single-marker'] })
  const allIds = []
  for (const f of features) {
    const ids = typeof f.properties.ids === 'string' ? JSON.parse(f.properties.ids) : f.properties.ids
    for (const id of ids) {
      if (!allIds.includes(id)) allIds.push(id)
    }
  }
  if (allIds.length === 0) return

  const BASE = import.meta.env.VITE_API_BASE || ''
  const label = allIds.length === 1 ? '1 Foto' : `${allIds.length} Fotos`
  const showAlbumsToggle = allIds.length > 12

  const toggleBtn = showAlbumsToggle
    ? `<button class="thumb-toggle-btn" data-action="albums">📁 Alben</button>`
    : ''

  popup = new maplibregl.Popup({ closeButton: false, maxWidth: '294px', offset: 12, anchor: getBestAnchor(mapInstance, point) })
    .setLngLat(lngLat)
    .setHTML(`<div class="thumb-popup-inner">
      <div class="thumb-popup-header-row">
        ${toggleBtn}
        <button class="thumb-close-btn" data-action="close">✕</button>
      </div>
      <div class="thumb-popup-body" style="min-width:262px;">${buildThumbsHTML(allIds, BASE)}</div>
    </div>`)
    .addTo(mapInstance)

  let albumsCache = null
  let mode = 'thumbs'

  popup.getElement().addEventListener('click', async (e) => {
    // navigate to media
    const img = e.target.closest('img[data-id]')
    if (img) {
      popup.remove(); popup = null
      router.push(`/media/${img.dataset.id}?${mapStateQuery(mapInstance)}`)
      return
    }

    // navigate to album
    const albumRow = e.target.closest('[data-album-id]')
    if (albumRow) {
      popup.remove(); popup = null
      router.push(`/album/${albumRow.dataset.albumId}`)
      return
    }

    // close popup
    const action = e.target.closest('[data-action]')?.dataset.action
    if (action === 'close') {
      popup.remove(); popup = null
      return
    }
    if (action === 'albums' && mode !== 'albums') {
      mode = 'albums'
      const body = popup.getElement().querySelector('.thumb-popup-body')
      body.innerHTML = `<div class="thumb-albums-loading">Lade Alben…</div>`
      if (!albumsCache) {
        try { albumsCache = await api.getAlbumsByMediaIDs(allIds) } catch { albumsCache = [] }
      }
      body.innerHTML = buildAlbumsHTML(albumsCache, BASE)
      const toggleBtn = popup.getElement().querySelector('.thumb-toggle-btn')
      if (toggleBtn) { toggleBtn.textContent = '📷 Fotos'; toggleBtn.dataset.action = 'thumbs' }
      return
    }

    // toggle back to thumbs
    if (action === 'thumbs' && mode !== 'thumbs') {
      mode = 'thumbs'
      const body = popup.getElement().querySelector('.thumb-popup-body')
      body.innerHTML = buildThumbsHTML(allIds, BASE)
      const toggleBtn = popup.getElement().querySelector('.thumb-toggle-btn')
      if (toggleBtn) { toggleBtn.textContent = '📁 Alben'; toggleBtn.dataset.action = 'albums' }
    }
  })
}

function setupInteractions(mapInstance) {
  // Click cluster: zoom in
  mapInstance.on('click', 'cluster-circle', (e) => {
    const zoom = Math.min(mapInstance.getZoom() + 2, 18)
    mapInstance.flyTo({ center: e.features[0].geometry.coordinates, zoom })
  })

  // Click single marker: show thumbnail grid popup
  mapInstance.on('click', 'single-marker', (e) => {
    showThumbnailPopup(mapInstance, e.features[0].geometry.coordinates.slice(), e.point)
  })

  mapInstance.on('mouseenter', 'single-marker', () => { mapInstance.getCanvas().style.cursor = 'pointer' })
  mapInstance.on('mouseleave', 'single-marker', () => { mapInstance.getCanvas().style.cursor = '' })
  mapInstance.on('mouseenter', 'cluster-circle', () => { mapInstance.getCanvas().style.cursor = 'pointer' })
  mapInstance.on('mouseleave', 'cluster-circle', () => { mapInstance.getCanvas().style.cursor = '' })

  // Close popup on map move
  mapInstance.on('movestart', () => { if (popup) { popup.remove(); popup = null } })
}

onMounted(async () => {
  const q = route.query
  const center = (q.lng && q.lat) ? [parseFloat(q.lng), parseFloat(q.lat)] : [10, 51]
  const zoom   = q.zoom ? parseFloat(q.zoom) : 4

  const BASE = import.meta.env.VITE_API_BASE || ''
  const styleJson = await fetch(`${BASE}/api/map/style`).then(r => r.json())

  map = new maplibregl.Map({
    container: mapContainer.value,
    style: styleJson,
    center,
    zoom,
  })

  map.addControl(new maplibregl.NavigationControl(), 'top-right')

  map.on('load', () => {
    loadClusters(map)
    setupInteractions(map)
  })

  map.on('moveend', () => loadClusters(map))
  map.on('zoomend', () => loadClusters(map))
})

onUnmounted(() => {
  if (map) { map.remove(); map = null }
})
</script>

<style scoped>
.map-view {
  display: flex;
  flex-direction: column;
  height: 100vh;
  width: 100%;
}

.map-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 16px;
  background: #1e1e2e;
  border-bottom: 1px solid #313244;
  flex-shrink: 0;
}

.btn-back {
  background: #313244;
  border: none;
  color: #cdd6f4;
  padding: 6px 12px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}

.btn-back:hover {
  background: #45475a;
}

.map-title {
  color: #cdd6f4;
  font-size: 15px;
  font-weight: 500;
}

.map-container {
  flex: 1;
  width: 100%;
}
</style>

<style>
/* MapLibre popup content is injected outside scoped DOM — must be global */
.maplibregl-popup-content {
  border: 1px solid #3b82f6;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
  border-radius: 10px;
  padding: 0;
}

.thumb-popup-inner {
  padding: 8px;
}

.thumb-popup-header {
  font-size: 13px;
  font-weight: 600;
  color: #3b82f6;
  margin-bottom: 8px;
  letter-spacing: 0.02em;
}

.thumb-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.thumb-img {
  width: 88px;
  height: 88px;
  object-fit: cover;
  border-radius: 6px;
  cursor: pointer;
  flex-shrink: 0;
  border: 2px solid transparent;
  transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
}

.thumb-img:hover {
  transform: scale(1.08);
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.35);
  border-color: #3b82f6;
}

.thumb-more-badge {
  display: block;
  margin-top: 8px;
  padding: 3px 10px;
  background: #3b82f6;
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  border-radius: 999px;
  width: fit-content;
  margin-left: auto;
}

.thumb-popup-header-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.thumb-toggle-btn {
  font-size: 11px;
  font-weight: 600;
  padding: 3px 8px;
  border-radius: 999px;
  border: 1px solid #3b82f6;
  background: transparent;
  color: #3b82f6;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.thumb-toggle-btn:hover {
  background: #3b82f6;
  color: #fff;
}

.thumb-albums-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.thumb-album-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.15s ease;
}

.thumb-album-row:hover {
  background: rgba(59, 130, 246, 0.12);
}

.thumb-album-cover {
  width: 40px;
  height: 40px;
  border-radius: 4px;
  object-fit: cover;
  flex-shrink: 0;
}

.thumb-album-cover--empty {
  background: #313244;
}

.thumb-album-info {
  flex: 1;
  min-width: 0;
}

.thumb-album-name {
  font-size: 13px;
  font-weight: 500;
  color: #cdd6f4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.thumb-album-count {
  font-size: 11px;
  color: #3b82f6;
  font-weight: 600;
}

.thumb-albums-loading,
.thumb-albums-empty {
  font-size: 12px;
  color: #888;
  padding: 8px 0;
  text-align: center;
}

.thumb-close-btn {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #3b82f6;
  color: #fff;
  font-size: 12px;
  border: none;
  cursor: pointer;
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  transition: background 0.15s ease;
  margin-left: 4px;
}

.thumb-close-btn:hover {
  background: #2563eb;
}
</style>
