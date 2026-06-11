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

// Popup for single images with thumbnail
let popup = null

function setupInteractions(mapInstance) {
  // Click on cluster: zoom in
  mapInstance.on('click', 'cluster-circle', (e) => {
    const zoom = Math.min(mapInstance.getZoom() + 2, 18)
    const coords = e.features[0].geometry.coordinates
    mapInstance.flyTo({ center: coords, zoom })
  })

  // Click on single marker: open lightbox, pass map state for back-navigation
  mapInstance.on('click', 'single-marker', (e) => {
    const id = e.features[0].properties.ids
    const ids = typeof id === 'string' ? JSON.parse(id) : id
    if (ids && ids.length > 0) {
      const center = mapInstance.getCenter()
      const zoom = mapInstance.getZoom()
      const q = new URLSearchParams({
        from: 'map',
        lat: center.lat.toFixed(6),
        lng: center.lng.toFixed(6),
        zoom: zoom.toFixed(2),
      })
      if (props.albumId) q.set('album_id', props.albumId)
      router.push(`/media/${ids[0]}?${q}`)
    }
  })

  // Hover on single marker: show thumbnail popup
  mapInstance.on('mouseenter', 'single-marker', (e) => {
    mapInstance.getCanvas().style.cursor = 'pointer'
    const props = e.features[0].properties
    const ids = typeof props.ids === 'string' ? JSON.parse(props.ids) : props.ids
    const thumbnailId = props.thumbnailId
    const coords = e.features[0].geometry.coordinates.slice()

    const BASE = import.meta.env.VITE_API_BASE || ''
    popup = new maplibregl.Popup({ closeButton: false, offset: 15 })
      .setLngLat(coords)
      .setHTML(`<img src="${BASE}/api/media/${thumbnailId}/thumbnail?size=320" style="width:200px;height:200px;object-fit:cover;border-radius:4px;" />`)
      .addTo(mapInstance)
  })

  mapInstance.on('mouseleave', 'single-marker', () => {
    mapInstance.getCanvas().style.cursor = ''
    if (popup) { popup.remove(); popup = null }
  })

  mapInstance.on('mouseenter', 'cluster-circle', () => {
    mapInstance.getCanvas().style.cursor = 'pointer'
  })
  mapInstance.on('mouseleave', 'cluster-circle', () => {
    mapInstance.getCanvas().style.cursor = ''
  })
}

onMounted(() => {
  const q = route.query
  const center = (q.lng && q.lat) ? [parseFloat(q.lng), parseFloat(q.lat)] : [10, 51]
  const zoom   = q.zoom ? parseFloat(q.zoom) : 4

  map = new maplibregl.Map({
    container: mapContainer.value,
    style: 'https://tiles.openfreemap.org/styles/liberty',
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
