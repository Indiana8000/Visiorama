<template>
  <nav class="breadcrumbs" aria-label="breadcrumb">
    <template v-for="(crumb, i) in crumbs" :key="i">
      <span v-if="i > 0" class="sep" aria-hidden="true">/</span>
      <router-link
        v-if="crumb.albumId != null"
        :to="{ name: 'album', params: { id: crumb.albumId } }"
        class="crumb"
      >{{ crumb.name }}</router-link>
      <router-link
        v-else-if="crumb.relativePath === ''"
        :to="{ name: 'root' }"
        class="crumb"
      >{{ crumb.name }}</router-link>
      <span v-else class="crumb crumb--current" aria-current="page">{{ crumb.name }}</span>
    </template>
  </nav>
</template>

<script setup>
defineProps({
  crumbs: {
    type: Array,
    default: () => [],
  },
})
</script>

<style scoped>
.breadcrumbs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  color: var(--muted);
  margin-bottom: 16px;
}
.sep { color: var(--border); }
.crumb { color: var(--muted); }
.crumb:hover { color: var(--accent); }
.crumb--current { color: var(--text); font-weight: 500; }
</style>
