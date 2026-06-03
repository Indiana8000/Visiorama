import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api/client.js'

export const useGalleryStore = defineStore('gallery', () => {
  const currentAlbum = ref(null)       // AlbumResponse: { album, breadcrumbs, childAlbums, media, page }
  const currentMedia = ref(null)       // MediaMetadata
  const loading = ref(false)
  const error = ref(null)

  async function fetchAlbum(id, page = 1, pageSize = 100) {
    loading.value = true
    error.value = null
    try {
      const result = id == null
        ? await api.getAlbumRoot(page, pageSize)
        : await api.getAlbumById(id, page, pageSize)
      console.log('[gallery] fetchAlbum response', JSON.stringify(result, null, 2))
      currentAlbum.value = result
    } catch (e) {
      error.value = e.message || 'Failed to load album'
      currentAlbum.value = null
    } finally {
      loading.value = false
    }
  }

  async function fetchMediaMetadata(mediaId) {
    loading.value = true
    error.value = null
    try {
      currentMedia.value = await api.getMediaMetadata(mediaId)
    } catch (e) {
      error.value = e.message || 'Failed to load media'
      currentMedia.value = null
    } finally {
      loading.value = false
    }
  }

  function clearError() {
    error.value = null
  }

  return {
    currentAlbum,
    currentMedia,
    loading,
    error,
    fetchAlbum,
    fetchMediaMetadata,
    clearError,
  }
})
