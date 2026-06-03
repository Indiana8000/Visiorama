<template>
  <router-link :to="{ name: 'album', params: { id: album.id } }" class="album-tile">
    <div class="album-tile__cover">
      <img
        v-if="album.coverThumbnailUrl"
        :src="coverSrc"
        :alt="album.name"
        loading="lazy"
        class="album-tile__img"
      />
      <div v-else class="album-tile__placeholder">
        <span class="album-tile__icon">&#128193;</span>
      </div>
    </div>
    <div class="album-tile__info">
      <span class="album-tile__name" :title="album.name">{{ album.name }}</span>
      <span class="album-tile__count">
        {{ album.mediaCountRecursive }} items
        <template v-if="album.childAlbumCount > 0">
          &middot; {{ album.childAlbumCount }} sub-album{{ album.childAlbumCount !== 1 ? 's' : '' }}
        </template>
      </span>
    </div>
  </router-link>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  album: {
    type: Object,
    required: true,
  },
})

const BASE = import.meta.env.VITE_API_BASE || ''

const coverSrc = computed(() => {
  const url = props.album.coverThumbnailUrl
  if (!url) return null
  // URL may already be absolute path like /api/media/88/thumbnail
  return url.startsWith('http') ? url : `${BASE}${url}`
})
</script>

<style scoped>
.album-tile {
  display: flex;
  flex-direction: column;
  background: var(--bg2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
  cursor: pointer;
  transition: border-color 0.15s, transform 0.15s;
  text-decoration: none;
  color: inherit;
}
.album-tile:hover {
  border-color: var(--accent);
  transform: translateY(-2px);
  text-decoration: none;
}

.album-tile__cover {
  width: 100%;
  aspect-ratio: 4 / 3;
  background: var(--bg3);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}
.album-tile__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.album-tile__placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
}
.album-tile__icon { font-size: 36px; }

.album-tile__info {
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.album-tile__name {
  font-weight: 600;
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.album-tile__count { font-size: 11px; color: var(--muted); }
</style>
