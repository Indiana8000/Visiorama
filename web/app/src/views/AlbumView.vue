<template>
  <div class="album-view">
    <div v-if="store.loading" class="state-msg">Loading…</div>
    <div v-else-if="store.error" class="state-msg state-msg--error">
      {{ store.error }}
    </div>
    <template v-else-if="store.currentAlbum">
      <p class="album-meta">
        {{ store.currentAlbum.album.mediaCountRecursive }} total items
        <template v-if="store.currentAlbum.childAlbums.length > 0">
          &middot; {{ store.currentAlbum.childAlbums.length }} album{{ store.currentAlbum.childAlbums.length !== 1 ? 's' : '' }}
        </template>
      </p>

      <!-- Child albums -->
      <section v-if="store.currentAlbum.childAlbums.length > 0" class="section">
        <h2 class="section-title">Albums</h2>
        <div class="grid grid--albums">
          <AlbumTile
            v-for="album in store.currentAlbum.childAlbums"
            :key="album.id"
            :album="album"
          />
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

      <!-- Empty state: only show scan button on root, not in sub-albums -->
      <div
        v-if="store.currentAlbum.childAlbums.length === 0 && store.currentAlbum.media.length === 0"
        class="empty-state"
      >
        <template v-if="store.currentAlbum.album.relativePath === ''">
          <p class="empty-state__msg">No media found. Run a scan to index your library.</p>
          <ScanButton @done="load(1)" />
        </template>
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
import { computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGalleryStore } from '../stores/gallery.js'
import AlbumTile from '../components/AlbumTile.vue'
import MediaTile from '../components/MediaTile.vue'
import ScanButton from '../components/ScanButton.vue'

const props = defineProps({
  id: { type: String, default: null },
})

const route = useRoute()
const router = useRouter()
const store = useGalleryStore()

const albumId = computed(() => props.id ? parseInt(props.id, 10) : null)
const pageInfo = computed(() => store.currentAlbum?.page ?? { page: 1, totalPages: 1, hasPrev: false, hasNext: false })

function load(page = 1) {
  store.fetchAlbum(albumId.value, page)
}

function changePage(page) {
  load(page)
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(() => load())
watch(() => route.params.id, () => load())
</script>

<style scoped>
.album-view { padding-bottom: 40px; }

.album-meta {
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
.grid--media {
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
}

@media (max-width: 480px) {
  .grid--albums { grid-template-columns: repeat(2, 1fr); }
  .grid--media { grid-template-columns: repeat(3, 1fr); }
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
</style>
