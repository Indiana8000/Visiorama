<template>
  <div id="app-shell">
    <header class="app-header">
      <router-link to="/" class="app-logo">Visiorama</router-link>
      <ScanButton @done="onScanDone" />
    </header>
    <main class="app-main">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router'
import { useGalleryStore } from './stores/gallery.js'
import ScanButton from './components/ScanButton.vue'

const router = useRouter()
const store = useGalleryStore()

function onScanDone() {
  // re-fetch whichever album is currently displayed
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

.app-logo {
  font-size: 18px;
  font-weight: 700;
  color: var(--text);
  letter-spacing: 1px;
}
.app-logo:hover { text-decoration: none; color: var(--accent); }

.app-main {
  max-width: 1400px;
  margin: 0 auto;
  padding: 20px;
}

@media (max-width: 600px) {
  .app-main { padding: 12px; }
}
</style>
