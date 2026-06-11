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

function showThumbnailPopup(mapInstance, lngLat, point) {
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
  const MAX = 12
  const shown = allIds.slice(0, MAX)
  const more = allIds.length > MAX
    ? `<div style="font-size:12px;color:#888;padding:4px 2px;">+${allIds.length - MAX} more</div>`
    : ''

  const imgs = shown.map(id =>
    `<img data-id="${id}" src="${BASE}/api/media/${id}/thumbnail?size=320"
      style="width:88px;height:88px;object-fit:cover;border-radius:6px;cursor:pointer;flex-shrink:0;" />`
  ).join('')

  popup = new maplibregl.Popup({ closeButton: true, maxWidth: '320px', offset: 12 })
    .setLngLat(lngLat)
    .setHTML(`<div style="display:flex;flex-wrap:wrap;gap:6px;padding:4px;">${imgs}${more}</div>`)
    .addTo(mapInstance)

  popup.getElement().addEventListener('click', (e) => {
    const img = e.target.closest('img[data-id]')
    if (!img) return
    popup.remove(); popup = null
    router.push(`/media/${img.dataset.id}?${mapStateQuery(mapInstance)}`)
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

  // MapLibre requires absolute sprite URLs
  if (typeof styleJson.sprite === 'string' && styleJson.sprite.startsWith('/')) {
    styleJson.sprite = window.location.origin + styleJson.sprite
  } else if (Array.isArray(styleJson.sprite)) {
    styleJson.sprite = styleJson.sprite.map(s =>
      typeof s.url === 'string' && s.url.startsWith('/')
        ? { ...s, url: window.location.origin + s.url }
        : s
    )
  }

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
