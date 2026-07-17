<template>
  <div class="lightbox-view" :class="{ 'lb-slideshow-fullscreen': slideshowActive }">
    <div v-if="store.loading" class="state-msg">Loading…</div>
    <div v-else-if="store.error" class="state-msg state-msg--error">{{ store.error }}</div>
    <template v-else-if="media">
      <!-- Nav bar -->
      <div class="lb-topbar">
        <button class="lb-back" @click="goBack">&#8592; {{ backLabel }}</button>
        <span class="lb-filename">{{ media.filename }}</span>
        <button
          v-if="slideshowImages.length > 1 && !slideshowActive"
          class="lb-slideshow-btn"
          @click="startSlideshow"
        >&#9654; Slideshow</button>
      </div>

      <!-- Media viewer -->
      <div class="lb-stage" @mousemove="slideshowActive && scheduleSsControlsHide()">
        <!-- Prev/Next navigation (hidden during slideshow) -->
        <button
          v-if="prevMedia && !slideshowActive"
          class="lb-nav lb-nav--prev"
          :title="prevMedia.filename"
          @click="navigate(prevMedia.id)"
        >&#8249;</button>

        <div class="lb-content">
          <div
            v-if="media.type === 'image'"
            ref="imgWrap"
            class="lb-img-wrap"
            :class="{ 'lb-img-wrap--zoomed': zoomScale > 1, 'lb-img-wrap--panning': isZoomPanning }"
            @wheel.prevent="onWheel"
            @mousedown="onMousedown"
            @mousemove="onMousemove"
            @mouseup="onMouseup"
            @mouseleave="onMouseup"
            @click="onImgWrapClick"
            @touchstart="onTouchstart"
            @touchmove="onTouchmove"
            @touchend="onTouchend"
          >
            <transition :name="slideshowActive ? 'lb-slide' : ''">
              <img
                v-show="slideshowActive ? (slideshowReady && !slideshowConvertFailed) : !imgConvertFailed"
                :key="slideshowActive ? 'ss-' + slideshowImgId + '-' + slideshowUseConvert : 'img-' + media.id + '-' + imgUseConvert"
                :src="slideshowActive ? slideshowImgSrc : imgSrc"
                :alt="media.filename"
                class="lb-img"
                :class="{ 'lb-img--warn': media.warningLargeMedia }"
                :style="!slideshowActive ? imgTransformStyle : undefined"
                draggable="false"
                @error="onImgError"
                @load="onImgLoad"
              />
            </transition>
            <span v-if="imgUseConvert && imgConvertLoaded && !imgConvertFailed" class="lb-img-badge">&#9432; Reduced quality</span>
            <div v-if="imgConvertFailed" class="lb-missing">
              <span class="lb-missing-icon">&#128247;</span>
              <span class="lb-missing-title">Image not found</span>
              <span class="lb-missing-hint">The file may have been deleted. A cleanup scan has been started automatically.</span>
            </div>
          </div>
          <div v-else-if="media.type === 'video'" class="lb-video-wrap">
            <video
              :key="'vid-' + media.id + '-' + transcodeStatus"
              :src="streamSrc"
              :poster="thumbSrc"
              :width="media.width || undefined"
              :height="media.height || undefined"
              controls
              class="lb-video"
              preload="metadata"
              :autoplay="transcodeStatus === 'success'"
              @error="onVideoError"
              @loadedmetadata="onVideoMetadata"
            ></video>
            <div v-if="transcodeError && transcodeStatus === null" class="lb-transcode-prompt">
              <span>&#9888; This format is not supported by your browser.</span>
              <button class="lb-transcode-btn" @click="startTranscode">&#128257; Convert to MP4</button>
            </div>
            <div v-if="transcodeStatus === 'queued' || transcodeStatus === 'running'" class="lb-transcode-prompt lb-transcode-prompt--progress">
              <span>&#9696; Converting…</span>
            </div>
            <div v-if="transcodeStatus === 'failed'" class="lb-transcode-prompt lb-transcode-prompt--error">
              <span>&#10005; Conversion failed: {{ transcodeErrMsg }}</span>
              <button class="lb-transcode-btn" @click="startTranscode">Retry</button>
            </div>
          </div>
        </div>

        <button
          v-if="nextMedia && !slideshowActive"
          class="lb-nav lb-nav--next"
          :title="nextMedia.filename"
          @click="navigate(nextMedia.id)"
        >&#8250;</button>

        <!-- Slideshow preload -->
        <template v-if="slideshowActive && slideshowNextId">
          <img :src="slideshowNextSrc" class="lb-preload" aria-hidden="true" />
          <img :src="slideshowNextConvertSrc" class="lb-preload" aria-hidden="true" />
        </template>

        <!-- Slideshow controls -->
        <div
          v-if="slideshowActive"
          class="lb-ss-controls"
          :class="{ 'lb-ss-controls--hidden': ssControlsHidden }"
          @mouseenter="onSsControlsMouseenter"
          @mouseleave="onSsControlsMouseleave"
        >
          <button class="lb-ss-ctrl-btn" :title="slideshowPaused ? 'Resume' : 'Pause'" @click="toggleSlideshowPause">
            <span v-if="slideshowPaused">&#9654;</span>
            <span v-else>&#9208;</span>
          </button>
          <button class="lb-ss-ctrl-btn lb-ss-ctrl-btn--stop" title="Stop slideshow" @click="stopSlideshow">&#9209;</button>
        </div>
      </div>

      <!-- Large file warning -->
      <div v-if="media.warningLargeMedia && !slideshowActive" class="lb-warning">
        &#9888; This file is large and may take time to load.
      </div>

      <!-- Metadata panel -->
      <div v-if="!slideshowActive" class="lb-meta">
        <h2 class="lb-meta-title">Details</h2>
        <dl class="lb-meta-list">
          <template v-if="media.captureDate">
            <dt>Date</dt>
            <dd>{{ formatDate(media.captureDate) }}</dd>
          </template>
          <template v-if="media.width && media.height">
            <dt>Dimensions</dt>
            <dd>{{ media.width }} &times; {{ media.height }}</dd>
          </template>
          <template v-if="media.durationMs">
            <dt>Duration</dt>
            <dd>{{ formatDuration(media.durationMs) }}</dd>
          </template>
          <dt>Size</dt>
          <dd>{{ formatSize(media.sizeBytes) }}</dd>
          <template v-if="media.mimeType">
            <dt>Type</dt>
            <dd>{{ media.mimeType }}</dd>
          </template>
          <template v-if="media.cameraModel">
            <dt>Camera</dt>
            <dd>{{ media.cameraModel }}</dd>
          </template>
          <template v-if="media.lensModel">
            <dt>Lens</dt>
            <dd>{{ media.lensModel }}</dd>
          </template>
          <template v-if="media.gpsLat != null && media.gpsLon != null">
            <dt>GPS</dt>
            <dd>{{ media.gpsLat.toFixed(5) }}, {{ media.gpsLon.toFixed(5) }}</dd>
          </template>
        </dl>

        <!-- AI persons -->
        <template v-if="aiFaces.some(f => f.personId)">
          <h3 class="lb-meta-section">Persons</h3>
          <div class="lb-persons">
            <router-link
              v-for="face in aiFaces.filter(f => f.personId)"
              :key="face.faceId"
              :to="{ name: 'person', params: { personId: face.personId } }"
              class="lb-person-chip"
            >
              <img v-if="face.cropPath" :src="face.cropPath" class="lb-person-chip__crop" />
              <span v-else class="lb-person-chip__icon">👤</span>
              <span class="lb-person-chip__name">{{ face.personName }}</span>
            </router-link>
          </div>
        </template>

        <!-- AI labels -->
        <template v-if="aiLabels.length > 0">
          <h3 class="lb-meta-section">Labels</h3>
          <div class="lb-labels">
            <span
              v-for="label in aiLabels"
              :key="label.label"
              class="lb-label-chip"
              :title="`${Math.round(label.confidence * 100)}% confidence`"
            >{{ label.label }}</span>
          </div>
        </template>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGalleryStore } from '../stores/gallery.js'
import { api } from '../api/client.js'

const SLIDESHOW_INTERVAL = 4000

const props = defineProps({
  id: { type: String, required: true },
})

const route = useRoute()
const router = useRouter()
const store = useGalleryStore()

const mediaId = computed(() => parseInt(route.params.id, 10))
const media = computed(() => store.currentMedia)

// --- Existing image/video state ---
const imgConvertFailed = ref(false)
const imgUseConvert = ref(false)
const imgConvertLoaded = ref(false)
const transcodeError = ref(false)
const transcodeJobId = ref(null)
const transcodeStatus = ref(null)
const transcodeErrMsg = ref(null)
let transcodePoller = null

// --- Zoom state ---
const imgWrap = ref(null)
const zoomScale = ref(1)
const panX = ref(0)
const panY = ref(0)
const isZoomPanning = ref(false)
let wasDragging = false
let panStart = { x: 0, y: 0 }
let pinchStartDist = 0
let pinchStartScale = 1

const imgTransformStyle = computed(() => ({
  transform: `translate(${panX.value}px, ${panY.value}px) scale(${zoomScale.value})`,
  transformOrigin: 'center center',
}))

// --- Slideshow state ---
const slideshowActive = ref(false)
const slideshowPaused = ref(false)
let slideshowTimer = null
const ssControlsHidden = ref(false)
let ssControlsHideTimer = null
const slideshowIdx = ref(0)
const slideshowUseConvert = ref(false)
const slideshowConvertFailed = ref(false)
const slideshowReady = ref(false)

// --- Computed ---
const imgSrc = computed(() => {
  if (!media.value) return ''
  if (imgUseConvert.value) return api.convertUrl(media.value.id)
  return api.streamUrl(media.value.id)
})

const streamSrc = computed(() => {
  if (transcodeStatus.value === 'success' && media.value) {
    return api.transcodeStreamUrl(media.value.id)
  }
  return media.value ? api.streamUrl(media.value.id) : ''
})
const thumbSrc = computed(() => media.value ? api.thumbnailUrl(media.value.id, 480) : '')

// Person-context siblings — loaded when navigating from a person album.
const personSiblings = ref([])
const personSiblingsLoaded = ref(false)

async function loadPersonSiblings(personId) {
  if (!personId) return
  personSiblings.value = []
  personSiblingsLoaded.value = false
  try {
    // Fetch all pages so prev/next works across the full person album.
    let page = 1
    const all = []
    while (true) {
      const res = await api.getPersonMedia(parseInt(personId, 10), page, 200)
      all.push(...(res.media ?? []))
      if (!res.page?.hasNext) break
      page++
    }
    personSiblings.value = all
  } catch { /* ignore */ }
  personSiblingsLoaded.value = true
}

const siblings = computed(() =>
  fromPersons.value ? personSiblings.value : (store.currentAlbum?.media ?? [])
)
const siblingIndex = computed(() => siblings.value.findIndex(m => m.id === mediaId.value))
const prevMedia = computed(() => siblingIndex.value > 0 ? siblings.value[siblingIndex.value - 1] : null)
const nextMedia = computed(() => {
  const idx = siblingIndex.value
  return idx >= 0 && idx < siblings.value.length - 1 ? siblings.value[idx + 1] : null
})

const slideshowImages = computed(() => siblings.value.filter(m => m.type === 'image'))

const slideshowImgId = computed(() => slideshowImages.value[slideshowIdx.value]?.id ?? null)
const slideshowImgSrc = computed(() => {
  const id = slideshowImgId.value
  if (!id) return ''
  return slideshowUseConvert.value ? api.convertUrl(id) : api.streamUrl(id)
})
const slideshowNextId = computed(() => {
  if (!slideshowActive.value) return null
  const images = slideshowImages.value
  if (images.length < 2) return null
  return images[(slideshowIdx.value + 1) % images.length]?.id ?? null
})
const slideshowNextSrc = computed(() => slideshowNextId.value ? api.streamUrl(slideshowNextId.value) : null)
const slideshowNextConvertSrc = computed(() => slideshowNextId.value ? api.convertUrl(slideshowNextId.value) : null)

const fromMap = computed(() => route.query.from === 'map')
const fromPersons = computed(() => route.query.from === 'persons')
const backLabel = computed(() => {
  if (fromMap.value) return 'Back to map'
  if (fromPersons.value) return route.query.personId ? 'Back to person' : 'Back to persons'
  return 'Back to album'
})

// --- Navigation ---
function navigate(id) {
  const q = (fromMap.value || fromPersons.value) ? { ...route.query } : {}
  router.push({ name: 'media', params: { id }, query: q })
}

function goBack() {
  if (fromMap.value) {
    const q = route.query
    const params = new URLSearchParams()
    if (q.lat)      params.set('lat', q.lat)
    if (q.lng)      params.set('lng', q.lng)
    if (q.zoom)     params.set('zoom', q.zoom)
    if (q.album_id) params.set('album_id', q.album_id)
    router.push(`/map?${params}`)
  } else if (fromPersons.value) {
    const pid = route.query.personId
    if (pid) {
      router.push({ name: 'person', params: { personId: pid } })
    } else {
      router.push({ name: 'persons' })
    }
  } else if (media.value?.albumId != null) {
    router.push({ name: 'album', params: { id: media.value.albumId } })
  } else {
    router.back()
  }
}

// --- Formatters ---
function formatDate(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleString()
}
function formatSize(bytes) {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = bytes, i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(1)} ${units[i]}`
}
function formatDuration(ms) {
  if (!ms) return ''
  const s = Math.round(ms / 1000)
  const m = Math.floor(s / 60)
  const h = Math.floor(m / 60)
  if (h > 0) return `${h}h ${m % 60}m ${s % 60}s`
  if (m > 0) return `${m}m ${s % 60}s`
  return `${s}s`
}

// --- Image loading ---
function onImgLoad() {
  if (imgUseConvert.value) imgConvertLoaded.value = true
}

function onImgError(e) {
  if (slideshowActive.value) {
    const id = slideshowImgId.value
    if (!id) return
    if (slideshowUseConvert.value) {
      const expected = new URL(api.convertUrl(id), window.location.href).href
      if (e.target.src === expected) slideshowConvertFailed.value = true
    } else {
      slideshowUseConvert.value = true
    }
    return
  }
  if (imgUseConvert.value) {
    const expectedSrc = new URL(api.convertUrl(media.value.id), window.location.href).href
    if (e.target.src === expectedSrc) {
      imgConvertFailed.value = true
    }
    return
  }
  imgUseConvert.value = true
}

function onVideoError() {
  if (transcodeStatus.value === 'success') return
  transcodeError.value = true
}

function onVideoMetadata(e) {
  if (transcodeStatus.value === 'success') return
  const vid = e.target
  if (vid.videoWidth === 0 && vid.duration > 0) {
    transcodeError.value = true
  }
}

// --- Transcode ---
async function startTranscode() {
  transcodeStatus.value = 'queued'
  transcodeErrMsg.value = null
  try {
    const res = await api.triggerTranscode(media.value.id)
    transcodeJobId.value = res.jobId
    pollTranscode()
  } catch (e) {
    transcodeStatus.value = 'failed'
    transcodeErrMsg.value = e.message
  }
}

function pollTranscode() {
  clearInterval(transcodePoller)
  let retries = 0
  const MAX_RETRIES = 150 // 5 minutes at 2s interval
  transcodePoller = setInterval(async () => {
    retries++
    if (retries > MAX_RETRIES) {
      transcodeStatus.value = 'failed'
      transcodeErrMsg.value = 'Timed out waiting for transcode'
      clearInterval(transcodePoller)
      return
    }
    try {
      const job = await api.getTranscodeStatus(transcodeJobId.value)
      transcodeStatus.value = job.status
      if (job.status === 'failed') {
        transcodeErrMsg.value = job.error || 'Unknown error'
        clearInterval(transcodePoller)
      } else if (job.status === 'success') {
        clearInterval(transcodePoller)
      }
    } catch {
      clearInterval(transcodePoller)
    }
  }, 2000)
}

function resetTranscodeState() {
  clearInterval(transcodePoller)
  imgUseConvert.value = false
  imgConvertFailed.value = false
  imgConvertLoaded.value = false
  transcodeError.value = false
  transcodeJobId.value = null
  transcodeStatus.value = null
  transcodeErrMsg.value = null
}

// --- Zoom ---
function resetZoom() {
  zoomScale.value = 1
  panX.value = 0
  panY.value = 0
}

function onImgWrapClick() {
  if (wasDragging) { wasDragging = false; return }
  if (slideshowActive.value) return
  if (zoomScale.value !== 1) {
    resetZoom()
  } else {
    zoomScale.value = 2
  }
}

function onWheel(e) {
  if (slideshowActive.value) return
  const factor = e.deltaY > 0 ? 0.85 : 1.18
  const newScale = Math.max(1, Math.min(10, zoomScale.value * factor))

  if (newScale === 1) {
    zoomScale.value = 1
    panX.value = 0
    panY.value = 0
    return
  }

  const wrap = imgWrap.value
  if (wrap) {
    const rect = wrap.getBoundingClientRect()
    const relX = e.clientX - (rect.left + rect.width / 2)
    const relY = e.clientY - (rect.top + rect.height / 2)
    const ratio = newScale / zoomScale.value
    panX.value = relX + (panX.value - relX) * ratio
    panY.value = relY + (panY.value - relY) * ratio
  }
  zoomScale.value = newScale
}

function onMousedown(e) {
  if (zoomScale.value <= 1 || slideshowActive.value) return
  isZoomPanning.value = true
  wasDragging = false
  panStart = { x: e.clientX - panX.value, y: e.clientY - panY.value }
  e.preventDefault()
}

function onMousemove(e) {
  if (!isZoomPanning.value) return
  wasDragging = true
  panX.value = e.clientX - panStart.x
  panY.value = e.clientY - panStart.y
}

function onMouseup() {
  isZoomPanning.value = false
}

function getTouchDist(touches) {
  const dx = touches[0].clientX - touches[1].clientX
  const dy = touches[0].clientY - touches[1].clientY
  return Math.sqrt(dx * dx + dy * dy)
}

function onTouchstart(e) {
  if (slideshowActive.value) return
  if (e.touches.length === 2) {
    e.preventDefault()
    pinchStartDist = getTouchDist(e.touches)
    pinchStartScale = zoomScale.value
  } else if (e.touches.length === 1 && zoomScale.value > 1) {
    isZoomPanning.value = true
    wasDragging = false
    const t = e.touches[0]
    panStart = { x: t.clientX - panX.value, y: t.clientY - panY.value }
  }
}

function onTouchmove(e) {
  if (slideshowActive.value) return
  if (e.touches.length === 2) {
    e.preventDefault()
    const dist = getTouchDist(e.touches)
    const newScale = Math.max(1, Math.min(10, pinchStartScale * dist / pinchStartDist))
    zoomScale.value = newScale
    if (newScale === 1) { panX.value = 0; panY.value = 0 }
  } else if (isZoomPanning.value && e.touches.length === 1) {
    e.preventDefault()
    wasDragging = true
    const t = e.touches[0]
    panX.value = t.clientX - panStart.x
    panY.value = t.clientY - panStart.y
  }
}

function onTouchend() {
  isZoomPanning.value = false
}

// --- Slideshow ---
function scheduleSsControlsHide() {
  clearTimeout(ssControlsHideTimer)
  ssControlsHidden.value = false
  ssControlsHideTimer = setTimeout(() => { ssControlsHidden.value = true }, 3000)
}

function onSsControlsMouseenter() {
  clearTimeout(ssControlsHideTimer)
  ssControlsHidden.value = false
}

function onSsControlsMouseleave() {
  scheduleSsControlsHide()
}

function startSlideshow() {
  const images = slideshowImages.value
  if (images.length < 2) return
  const idx = images.findIndex(m => m.id === mediaId.value)
  slideshowIdx.value = idx === -1 ? 0 : idx
  slideshowReady.value = false
  slideshowActive.value = true
  slideshowPaused.value = false
  slideshowUseConvert.value = false
  slideshowConvertFailed.value = false
  nextTick(() => { slideshowReady.value = true })
  scheduleSlideshowTick()
  scheduleSsControlsHide()
}

function stopSlideshow() {
  slideshowActive.value = false
  slideshowPaused.value = false
  clearTimeout(slideshowTimer)
  slideshowTimer = null
  clearTimeout(ssControlsHideTimer)
  ssControlsHidden.value = false
}

function toggleSlideshowPause() {
  if (!slideshowActive.value) return
  slideshowPaused.value = !slideshowPaused.value
  if (!slideshowPaused.value) {
    scheduleSlideshowTick()
  } else {
    clearTimeout(slideshowTimer)
    slideshowTimer = null
  }
}

function scheduleSlideshowTick() {
  clearTimeout(slideshowTimer)
  slideshowTimer = setTimeout(slideshowAdvance, SLIDESHOW_INTERVAL)
}

function slideshowAdvance() {
  const images = slideshowImages.value
  if (!images.length) { stopSlideshow(); return }
  slideshowIdx.value = (slideshowIdx.value + 1) % images.length
  slideshowUseConvert.value = false
  slideshowConvertFailed.value = false
  scheduleSlideshowTick()
}

// --- AI data ---
const aiLabels = ref([])
const aiFaces = ref([])

async function loadAI(id) {
  try {
    const res = await api.getMediaAI(id)
    aiLabels.value = res.labels ?? []
    aiFaces.value = res.faces ?? []
  } catch {
    aiLabels.value = []
    aiFaces.value = []
  }
}

// --- Load ---
async function load(id) {
  if (!Number.isFinite(id) || id <= 0) return
  resetTranscodeState()
  resetZoom()
  aiLabels.value = []
  aiFaces.value = []
  await store.fetchMediaMetadata(id)
  const personId = route.query.personId
  if (personId) {
    // Load person siblings only once per person context (not on every navigate).
    if (!personSiblingsLoaded.value || personSiblings.value.length === 0) {
      loadPersonSiblings(personId)
    }
  } else if (store.currentMedia && store.currentAlbum?.album?.id !== store.currentMedia.albumId) {
    store.fetchAlbum(store.currentMedia.albumId, 1, 500)
  }
  loadAI(id)
}

// --- Keyboard ---
function onKey(e) {
  if (slideshowActive.value) {
    if (e.key === ' ' || e.key === 'Spacebar') {
      e.preventDefault()
      toggleSlideshowPause()
    }
    if (e.key === 'Escape') stopSlideshow()
    return
  }
  if (e.key === 'ArrowRight' && nextMedia.value) navigate(nextMedia.value.id)
  if (e.key === 'ArrowLeft'  && prevMedia.value) navigate(prevMedia.value.id)
  if (e.key === 'Escape') goBack()
}

onMounted(() => {
  load(mediaId.value)
  window.addEventListener('keydown', onKey)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
  clearInterval(transcodePoller)
  stopSlideshow()
})

watch(mediaId, (id) => load(id))

watch(() => route.query.personId, (newId, oldId) => {
  if (newId !== oldId) {
    personSiblings.value = []
    personSiblingsLoaded.value = false
  }
})
</script>

<style scoped>
.lightbox-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding-bottom: 40px;
}

/* Slideshow fullscreen mode */
.lb-slideshow-fullscreen {
  position: fixed;
  inset: 0;
  z-index: 200;
  background: #000;
  padding-bottom: 0;
  gap: 0;
}
.lb-slideshow-fullscreen .lb-topbar,
.lb-slideshow-fullscreen .lb-warning,
.lb-slideshow-fullscreen .lb-meta {
  display: none;
}
.lb-slideshow-fullscreen .lb-stage {
  flex: 1;
  min-height: 0;
  border-radius: 0;
  max-height: 100vh;
}
.lb-slideshow-fullscreen .lb-content {
  height: 100%;
  max-height: 100vh;
}
.lb-slideshow-fullscreen .lb-img-wrap {
  height: 100%;
}
.lb-slideshow-fullscreen .lb-img {
  max-height: 100vh;
}

/* Slide transition */
.lb-slide-enter-active,
.lb-slide-leave-active {
  transition: transform 0.35s ease;
}
.lb-slide-enter-from {
  transform: translateX(100%);
}
.lb-slide-leave-to {
  transform: translateX(-100%);
}
.lb-slide-leave-active {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.lb-topbar {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 4px;
}
.lb-back {
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--muted);
  font-size: 13px;
  padding: 5px 12px;
  cursor: pointer;
  transition: color 0.15s, border-color 0.15s;
  flex-shrink: 0;
}
.lb-back:hover { color: var(--accent); border-color: var(--accent); }
.lb-filename {
  font-size: 14px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}
.lb-slideshow-btn {
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--muted);
  font-size: 13px;
  padding: 5px 12px;
  cursor: pointer;
  transition: color 0.15s, border-color 0.15s;
  flex-shrink: 0;
  margin-left: auto;
}
.lb-slideshow-btn:hover { color: var(--accent); border-color: var(--accent); }

.lb-stage {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
  border-radius: var(--radius);
  overflow: hidden;
  min-height: 300px;
}

.lb-content {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 75vh;
}

.lb-img-wrap {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 75vh;
  overflow: hidden;
  cursor: zoom-in;
}
.lb-img-wrap--zoomed { cursor: grab; }
.lb-img-wrap--panning { cursor: grabbing; }
.lb-slideshow-fullscreen .lb-img-wrap { cursor: default; }
.lb-preload { display: none; }

.lb-ss-controls {
  position: absolute;
  bottom: 24px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 4px;
  background: rgba(0, 0, 0, 0.5);
  border-radius: var(--radius, 6px);
  padding: 6px 10px;
  z-index: 20;
  opacity: 0.8;
  transition: opacity 0.4s ease;
}
.lb-ss-controls--hidden {
  opacity: 0;
  pointer-events: none;
}
.lb-ss-ctrl-btn {
  background: none;
  border: none;
  color: #fff;
  font-size: 18px;
  line-height: 1;
  padding: 4px 8px;
  cursor: pointer;
  transition: color 0.15s;
}
.lb-ss-ctrl-btn:hover { color: var(--accent, #6ea8fe); }
.lb-ss-ctrl-btn--stop:hover { color: #f08080; }

.lb-img {
  max-width: 100%;
  max-height: 75vh;
  object-fit: contain;
  display: block;
  user-select: none;
  -webkit-user-drag: none;
  will-change: transform;
}
.lb-img--warn { opacity: 0.9; }

.lb-missing {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 48px 32px;
  color: var(--muted);
  text-align: center;
}
.lb-missing-icon { font-size: 48px; opacity: 0.35; }
.lb-missing-title { font-size: 15px; font-weight: 600; color: var(--fg); }
.lb-missing-hint { font-size: 12px; max-width: 280px; line-height: 1.5; }
.lb-img-badge {
  position: absolute;
  bottom: 8px;
  right: 8px;
  background: rgba(0,0,0,0.6);
  color: #ccc;
  font-size: 10px;
  padding: 2px 7px;
  border-radius: 3px;
  pointer-events: none;
}

.lb-video-wrap {
  position: relative;
  width: 100%;
}
.lb-video {
  width: 100%;
  max-height: 75vh;
  display: block;
  background: #000;
}
.lb-transcode-prompt {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  background: rgba(0,0,0,0.75);
  color: #fff;
  font-size: 13px;
  flex-wrap: wrap;
}
.lb-transcode-prompt--progress { color: #aaa; }
.lb-transcode-prompt--error { color: #f08080; }
.lb-transcode-btn {
  background: var(--accent);
  color: #fff;
  border: none;
  border-radius: var(--radius);
  padding: 4px 12px;
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
}
.lb-transcode-btn:hover { opacity: 0.85; }

.lb-nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  background: rgba(0,0,0,0.5);
  border: none;
  color: #fff;
  font-size: 36px;
  line-height: 1;
  padding: 10px 14px;
  cursor: pointer;
  z-index: 10;
  transition: background 0.15s;
  border-radius: 4px;
}
.lb-nav:hover { background: rgba(0,0,0,0.8); }
.lb-nav--prev { left: 8px; }
.lb-nav--next { right: 8px; }

.lb-warning {
  background: #3a2a00;
  border: 1px solid #7a5500;
  color: #f0c040;
  border-radius: var(--radius);
  padding: 8px 14px;
  font-size: 13px;
}

.lb-meta {
  background: var(--bg2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 16px;
}
.lb-meta-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 12px;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}
.lb-meta-list {
  display: grid;
  grid-template-columns: 120px 1fr;
  gap: 6px 12px;
}
.lb-meta-list dt { color: var(--muted); font-size: 12px; align-self: start; padding-top: 1px; }
.lb-meta-list dd { font-size: 13px; word-break: break-word; }

.state-msg {
  padding: 60px;
  text-align: center;
  color: var(--muted);
  font-size: 15px;
}
.state-msg--error { color: var(--danger); }

.lb-meta-section {
  font-size: 11px;
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin: 14px 0 8px;
}

/* Person chips */
.lb-persons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.lb-person-chip {
  display: flex;
  align-items: center;
  gap: 6px;
  background: var(--bg3);
  border: 1px solid var(--border);
  border-radius: 20px;
  padding: 4px 10px 4px 4px;
  text-decoration: none;
  color: var(--text);
  font-size: 13px;
  transition: border-color 0.15s;
}
.lb-person-chip:hover { border-color: var(--accent); text-decoration: none; }
.lb-person-chip__crop {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  object-fit: cover;
}
.lb-person-chip__icon { font-size: 20px; width: 28px; text-align: center; }
.lb-person-chip__name { font-weight: 500; }

/* Label chips */
.lb-labels {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.lb-label-chip {
  background: var(--bg3);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 3px 9px;
  font-size: 12px;
  color: var(--muted);
  cursor: default;
}

@media (max-width: 600px) {
  .lb-meta-list { grid-template-columns: 90px 1fr; }
}
</style>
