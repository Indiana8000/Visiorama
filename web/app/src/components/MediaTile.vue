<template>
  <router-link :to="{ name: 'media', params: { id: media.id } }" class="media-tile">
    <div class="media-tile__thumb">
      <img
        :src="thumbSrc"
        :alt="media.filename"
        loading="lazy"
        class="media-tile__img"
      />
      <span v-if="media.type === 'video'" class="media-tile__badge media-tile__badge--video">
        &#9654; video<template v-if="videoDuration">&nbsp;{{ videoDuration }}</template>
      </span>
    </div>
    <div class="media-tile__name" :title="media.filename">{{ media.filename }}</div>
  </router-link>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  media: {
    type: Object,
    required: true,
  },
})

const BASE = import.meta.env.VITE_API_BASE || ''

const thumbSrc = computed(() => {
  const url = props.media.thumbnailUrl
  if (!url) return ''
  return url.startsWith('http') ? url : `${BASE}${url}`
})

const videoDuration = computed(() => {
  const ms = props.media.durationMs
  if (!ms) return ''
  const s = Math.round(ms / 1000)
  const m = Math.floor(s / 60)
  const h = Math.floor(m / 60)
  if (h > 0) return `${h}h ${m % 60}m ${s % 60}s`
  if (m > 0) return `${m}m ${s % 60}s`
  return `${s}s`
})
</script>

<style scoped>
.media-tile {
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
.media-tile:hover {
  border-color: var(--accent);
  transform: translateY(-2px);
  text-decoration: none;
}

.media-tile__thumb {
  position: relative;
  width: 100%;
  aspect-ratio: 4 / 3;
  background: var(--bg3);
  overflow: hidden;
}
.media-tile__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.media-tile__badge {
  position: absolute;
  bottom: 4px;
  left: 4px;
  font-size: 10px;
  padding: 2px 5px;
  border-radius: 3px;
  font-weight: 600;
}
.media-tile__badge--video {
  background: rgba(0,0,0,0.7);
  color: #fff;
}

.media-tile__name {
  padding: 5px 8px;
  font-size: 11px;
  color: var(--muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
