<template>
  <div class="lightbox-view">
    <div v-if="store.loading" class="state-msg">Loading…</div>
    <div v-else-if="store.error" class="state-msg state-msg--error">{{ store.error }}</div>
    <template v-else-if="media">
      <!-- Nav bar -->
      <div class="lb-topbar">
        <button class="lb-back" @click="goBack">&#8592; Back to album</button>
        <span class="lb-filename">{{ media.filename }}</span>
      </div>

      <!-- Media viewer -->
      <div class="lb-stage">
        <!-- Prev/Next navigation (uses sibling media from store.currentAlbum if available) -->
        <button
          v-if="prevMedia"
          class="lb-nav lb-nav--prev"
          :title="prevMedia.filename"
          @click="navigate(prevMedia.id)"
        >&#8249;</button>

        <div class="lb-content">
          <img
            v-if="media.type === 'image'"
            :key="'img-' + media.id"
            :src="streamSrc"
            :alt="media.filename"
            class="lb-img"
            :class="{ 'lb-img--warn': media.warningLargeMedia }"
          />
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
              @error="onVideoError"
              @loadedmetadata="onVideoMetadata"
            ></video>
            <div v-if="transcodeError && transcodeStatus === null" class="lb-transcode-prompt">
              <span>&#9888; Browser kann dieses Format nicht abspielen.</span>
              <button class="lb-transcode-btn" @click="startTranscode">&#128257; Zu MP4 konvertieren</button>
            </div>
            <div v-if="transcodeStatus === 'queued' || transcodeStatus === 'running'" class="lb-transcode-prompt lb-transcode-prompt--progress">
              <span>&#9696; Konvertierung läuft…</span>
            </div>
            <div v-if="transcodeStatus === 'failed'" class="lb-transcode-prompt lb-transcode-prompt--error">
              <span>&#10005; Konvertierung fehlgeschlagen: {{ transcodeErrMsg }}</span>
              <button class="lb-transcode-btn" @click="startTranscode">Nochmal versuchen</button>
            </div>
          </div>
        </div>

        <button
          v-if="nextMedia"
          class="lb-nav lb-nav--next"
          :title="nextMedia.filename"
          @click="navigate(nextMedia.id)"
        >&#8250;</button>
      </div>

      <!-- Large file warning -->
      <div v-if="media.warningLargeMedia" class="lb-warning">
        &#9888; This file is large and may take time to load.
      </div>

      <!-- Metadata panel -->
      <div class="lb-meta">
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
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGalleryStore } from '../stores/gallery.js'
import { api } from '../api/client.js'

const props = defineProps({
  id: { type: String, required: true },
})

const route = useRoute()
const router = useRouter()
const store = useGalleryStore()

const BASE = import.meta.env.VITE_API_BASE || ''

// Use route.params directly — always in sync, no prop-update race
const mediaId = computed(() => parseInt(route.params.id, 10))
const media = computed(() => store.currentMedia)

const transcodeError = ref(false)
const transcodeJobId = ref(null)
const transcodeStatus = ref(null) // null | 'queued' | 'running' | 'success' | 'failed'
const transcodeErrMsg = ref(null)
let transcodePoller = null

const streamSrc = computed(() => {
  if (transcodeStatus.value === 'success' && media.value) {
    return api.transcodeStreamUrl(media.value.id)
  }
  return media.value ? api.streamUrl(media.value.id) : ''
})
const thumbSrc  = computed(() => media.value ? api.thumbnailUrl(media.value.id, 480) : '')

// Sibling navigation — load parent album if not already in store
const siblings = computed(() => store.currentAlbum?.media ?? [])
const siblingIndex = computed(() => siblings.value.findIndex(m => m.id === mediaId.value))
const prevMedia = computed(() => siblingIndex.value > 0 ? siblings.value[siblingIndex.value - 1] : null)
const nextMedia = computed(() => {
  const idx = siblingIndex.value
  return idx >= 0 && idx < siblings.value.length - 1 ? siblings.value[idx + 1] : null
})

function navigate(id) {
  router.push({ name: 'media', params: { id } })
}

function goBack() {
  if (media.value?.albumId != null) {
    router.push({ name: 'album', params: { id: media.value.albumId } })
  } else {
    router.back()
  }
}

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
  transcodePoller = setInterval(async () => {
    try {
      const job = await api.getTranscodeStatus(transcodeJobId.value)
      transcodeStatus.value = job.status
      if (job.status === 'failed') {
        transcodeErrMsg.value = job.error || 'Unbekannter Fehler'
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
  transcodeError.value = false
  transcodeJobId.value = null
  transcodeStatus.value = null
  transcodeErrMsg.value = null
}

async function load(id) {
  resetTranscodeState()
  await store.fetchMediaMetadata(id)
  // Load parent album for sibling nav if not already loaded or stale
  if (store.currentMedia && store.currentAlbum?.album?.id !== store.currentMedia.albumId) {
    store.fetchAlbum(store.currentMedia.albumId, 1, 500)
  }
}

onMounted(() => {
  load(mediaId.value)
  window.addEventListener('keydown', onKey)
})

function onKey(e) {
  if (e.key === 'ArrowRight' && nextMedia.value) navigate(nextMedia.value.id)
  if (e.key === 'ArrowLeft'  && prevMedia.value) navigate(prevMedia.value.id)
  if (e.key === 'Escape') goBack()
}

onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
  clearInterval(transcodePoller)
})
watch(mediaId, (id) => load(id))
</script>

<style scoped>
.lightbox-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding-bottom: 40px;
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
}
.lb-back:hover { color: var(--accent); border-color: var(--accent); }
.lb-filename {
  font-size: 14px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

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
  max-height: 75vh;
}

.lb-img {
  max-width: 100%;
  max-height: 75vh;
  object-fit: contain;
  display: block;
}
.lb-img--warn { opacity: 0.9; }

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

@media (max-width: 600px) {
  .lb-meta-list { grid-template-columns: 90px 1fr; }
}
</style>
