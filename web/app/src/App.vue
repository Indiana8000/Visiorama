<template>
  <div id="app-shell">
    <header class="app-header">
      <nav class="app-breadcrumbs" aria-label="breadcrumb">
        <router-link to="/" class="crumb crumb--root">Visiorama</router-link>
        <template v-for="(crumb, i) in breadcrumbs" :key="i">
          <span class="sep" aria-hidden="true">/</span>
          <router-link
            v-if="crumb.albumId != null && i < breadcrumbs.length - 1"
            :to="{ name: 'album', params: { id: crumb.albumId } }"
            class="crumb"
          >{{ crumb.name }}</router-link>
          <span
            v-else-if="crumb.albumId != null"
            class="crumb crumb--current crumb--reload"
            @click="onCurrentCrumbClick(crumb)"
          >{{ crumb.name }}</span>
          <span v-else class="crumb crumb--current">{{ crumb.name }}</span>
        </template>
      </nav>
      <ScanButton :albumPath="currentAlbumPath" @done="onScanDone" />
    </header>
    <main class="app-main">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useGalleryStore } from './stores/gallery.js'
import ScanButton from './components/ScanButton.vue'

const router = useRouter()
const route = useRoute()
const store = useGalleryStore()

const currentAlbumPath = computed(() => store.currentAlbum?.album?.relativePath ?? '')

// Skip root crumb — it's always the "Visiorama" logo link
const breadcrumbs = computed(() => {
  const crumbs = store.currentAlbum?.breadcrumbs ?? []
  return crumbs.filter(c => c.relativePath !== '')
})

function onCurrentCrumbClick(crumb) {
  router.push({ name: 'album', params: { id: crumb.albumId } })
}

function onScanDone() {
  const route = router.currentRoute.value
  const id = route.params.id ? parseInt(route.params.id, 10) : null
  store.fetchAlbum(id, 1)
}
</script>

<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
  --bg: #1a1a1a;
  --bg2: #242424;
  --bg3: #2e2e2e;
  --border: #3a3a3a;
  --text: #e8e8e8;
  --muted: #888;
  --accent: #4a9eff;
  --danger: #e05555;
  --success: #4caf50;
  --radius: 8px;
  --gap: 12px;
}

body {
  background: var(--bg);
  color: var(--text);
  font-family: system-ui, -apple-system, sans-serif;
  font-size: 14px;
  line-height: 1.5;
  min-height: 100vh;
}

a { color: var(--accent); text-decoration: none; }
a:hover { text-decoration: underline; }

.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 52px;
  background: var(--bg2);
  border-bottom: 1px solid var(--border);
  position: sticky;
  top: 0;
  z-index: 100;
}

.app-breadcrumbs {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 14px;
  min-width: 0;
  flex: 1;
  overflow: hidden;
}

@media (max-width: 480px) {
  .app-header {
    flex-wrap: wrap;
    height: auto;
    padding: 10px 16px;
    gap: 8px;
  }
  .app-breadcrumbs {
    flex-wrap: wrap;
    width: 100%;
    overflow: visible;
  }
}
.sep { color: var(--border); user-select: none; }
.crumb {
  color: var(--muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 200px;
  text-decoration: none;
}
.crumb:hover { color: var(--accent); text-decoration: none; }
.crumb--root {
  font-size: 16px;
  font-weight: 700;
  color: var(--text);
  letter-spacing: 0.5px;
  flex-shrink: 0;
}
.crumb--root:hover { color: var(--accent); }
.crumb--current {
  color: var(--text);
  font-weight: 500;
  cursor: default;
}
.crumb--reload {
  cursor: pointer;
}
.crumb--reload:hover {
  color: var(--accent);
}

.app-main {
  max-width: 1400px;
  margin: 0 auto;
  padding: 20px;
}

@media (max-width: 600px) {
  .app-main { padding: 12px; }
}
</style>
