<template>
  <div class="album-view">
    <div v-if="store.loading" class="state-msg">Loading…</div>
    <div v-else-if="store.error" class="state-msg state-msg--error">
      {{ store.error }}
    </div>
    <template v-else-if="store.currentAlbum">
      <div class="album-meta">
        <span>
          {{ store.currentAlbum.album.mediaCountRecursive.toLocaleString() }} items
          <template v-if="store.currentAlbum.childAlbums.length > 0">
            &middot; {{ store.currentAlbum.childAlbums.length.toLocaleString() }} album{{ store.currentAlbum.childAlbums.length !== 1 ? 's' : '' }}
          </template>
        </span>
        <div class="meta-buttons">
          <button v-if="gpsCount > 0" class="btn-map" @click="openMap">
            🗺 Map<span class="persons-badge">{{ gpsCount }}</span>
          </button>
          <button class="btn-persons" @click="$router.push('/persons')">
            👥 Persons<span v-if="pendingClusters > 0" class="persons-badge">{{ pendingClusters }}</span>
          </button>
        </div>
      </div>

      <!-- Child albums -->
      <section v-if="store.currentAlbum.childAlbums.length > 0" class="section">
        <h2 class="section-title">Albums</h2>
        <div class="grid grid--albums">
          <template v-for="(album, index) in store.currentAlbum.childAlbums" :key="album.id">
            <div
              v-if="index > 0 && /^\d/.test(store.currentAlbum.childAlbums[index - 1].name) && !/^\d/.test(album.name)"
              class="album-group-spacer"
            />
            <AlbumTile :album="album" />
          </template>
        </div>
      </section>

      <!-- Media items -->
      <section v-if="store.currentAlbum.media.length > 0" class="section">
        <h2 class="section-title">Media</h2>
        <div class="grid grid--media">
          <MediaTile
            v-for="item in store.currentAlbum.media"
            :key="item.id"
            :media="item"
          />
        </div>
      </section>

      <!-- Empty state -->
      <div
        v-if="store.currentAlbum.childAlbums.length === 0 && store.currentAlbum.media.length === 0"
        class="empty-state"
      >
        <p v-if="store.currentAlbum.album.relativePath === ''" class="empty-state__msg">No media found. Run a scan to index your library.</p>
        <p v-else class="empty-state__msg">This album is empty.</p>
      </div>

      <!-- Pagination -->
      <div v-if="pageInfo.totalPages > 1" class="pagination">
        <button
          class="page-btn"
          :disabled="!pageInfo.hasPrev"
          @click="changePage(pageInfo.page - 1)"
        >&#8592; Prev</button>
        <span class="page-info">Page {{ pageInfo.page }} / {{ pageInfo.totalPages }}</span>
        <button
          class="page-btn"
          :disabled="!pageInfo.hasNext"
          @click="changePage(pageInfo.page + 1)"
        >Next &#8594;</button>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGalleryStore } from '../stores/gallery.js'
import { api } from '../api/client.js'
import AlbumTile from '../components/AlbumTile.vue'
import MediaTile from '../components/MediaTile.vue'

const props = defineProps({
  id: { type: String, default: null },
})

const route = useRoute()
const router = useRouter()
const store = useGalleryStore()

const albumId = computed(() => props.id ? parseInt(props.id, 10) : null)
const pageInfo = computed(() => store.currentAlbum?.page ?? { page: 1, totalPages: 1, hasPrev: false, hasNext: false })
const gpsCount = ref(0)
const pendingClusters = ref(0)

async function loadAICounts() {
  try {
    const res = await api.getAICounts()
    pendingClusters.value = res.pendingClusters ?? 0
  } catch { /* sidecar may not be running */ }
}

async function loadGPSCount(id) {
  if (id == null) { gpsCount.value = 0; return }
  try {
    const res = await api.getAlbumGPSCount(id)
    gpsCount.value = res.count
  } catch { gpsCount.value = 0 }
}

function openMap() {
  const id = store.currentAlbum?.album?.id
  if (id != null && id !== 0) {
    router.push(`/map?album_id=${id}`)
  } else {
    router.push('/map')
  }
}

function load(page = 1) {
  store.fetchAlbum(albumId.value, page)
}

function changePage(page) {
  load(page)
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(() => { load(); loadAICounts() })
watch(() => route.params.id, () => { load(); loadAICounts() })
watch(() => route.name, (name) => {
  if (name === 'album' || name === 'root') loadAICounts()
})
watch(() => store.currentAlbum, (album) => {
  loadGPSCount(album?.album?.id ?? null)
})
</script>

<style scoped>
.album-view { padding-bottom: 40px; }

.album-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  color: var(--muted);
  margin-bottom: 20px;
}

.section { margin-bottom: 32px; }
.section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin-bottom: 12px;
}

.grid {
  display: grid;
  gap: var(--gap);
}
.grid--albums {
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
}
.album-group-spacer {
  grid-column: 1 / -1;
  height: 1rem;
}
.grid--media {
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
}

@media (max-width: 480px) {
  .grid--albums { grid-template-columns: repeat(2, 1fr); }
  .grid--media { grid-template-columns: repeat(2, 1fr); }
}

.pagination {
  display: flex;
  align-items: center;
  gap: 12px;
  justify-content: center;
  margin-top: 32px;
}
.page-btn {
  padding: 6px 16px;
  background: var(--bg2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--text);
  cursor: pointer;
  font-size: 13px;
  transition: background 0.15s;
}
.page-btn:hover:not(:disabled) { background: var(--bg3); }
.page-btn:disabled { opacity: 0.4; cursor: default; }
.page-info { font-size: 13px; color: var(--muted); }

.state-msg {
  padding: 40px;
  text-align: center;
  color: var(--muted);
  font-size: 15px;
}
.state-msg--error { color: var(--danger); }

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 60px 20px;
}
.empty-state__msg {
  color: var(--muted);
  font-size: 15px;
  text-align: center;
}

.meta-buttons { display: flex; gap: 8px; align-items: center; }

.btn-map {
  background: #313244;
  border: none;
  color: #cdd6f4;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
.btn-map:hover { background: #45475a; }

.btn-persons {
  background: #313244;
  border: none;
  color: #cdd6f4;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 6px;
}
.btn-persons:hover { background: #45475a; }
.persons-badge {
  background: var(--accent, #cba6f7);
  color: #1e1e2e;
  border-radius: 9px;
  padding: 0 6px;
  font-size: 11px;
  font-weight: 700;
  line-height: 16px;
}
</style>
