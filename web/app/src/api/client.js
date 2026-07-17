const BASE = import.meta.env.VITE_API_BASE || ''

async function request(path, options = {}) {
  const url = `${BASE}${path}`
  const res = await fetch(url, options)
  if (!res.ok) {
    let errBody
    try { errBody = await res.json() } catch { errBody = { code: 'UNKNOWN', message: res.statusText } }
    const err = new Error(errBody.message || res.statusText)
    err.code = errBody.code
    err.status = res.status
    throw err
  }
  // 204 No Content or 202 with body
  const ct = res.headers.get('content-type') || ''
  if (ct.includes('application/json')) return res.json()
  return res
}

function pageParams(page, pageSize) {
  const p = new URLSearchParams()
  if (page != null) p.set('page', page)
  if (pageSize != null) p.set('pageSize', pageSize)
  return p.toString() ? `?${p}` : ''
}

export const api = {
  /** GET /api/albums/root */
  getAlbumRoot(page, pageSize) {
    return request(`/api/albums/root${pageParams(page, pageSize)}`)
  },

  /** GET /api/albums/:id */
  getAlbumById(albumId, page, pageSize) {
    return request(`/api/albums/${albumId}${pageParams(page, pageSize)}`)
  },

  /** GET /api/albums/by-path?path=... */
  getAlbumByPath(path, page, pageSize) {
    const p = new URLSearchParams({ path })
    if (page != null) p.set('page', page)
    if (pageSize != null) p.set('pageSize', pageSize)
    return request(`/api/albums/by-path?${p}`)
  },

  /** GET /api/media/:id/metadata */
  getMediaMetadata(mediaId) {
    return request(`/api/media/${mediaId}/metadata`)
  },

  /** GET /api/media/:id/ai — labels and faces with person assignments */
  getMediaAI(mediaId) {
    return request(`/api/media/${mediaId}/ai`)
  },

  /** GET /api/media/:id/thumbnail?size=... */
  thumbnailUrl(mediaId, size) {
    const qs = size ? `?size=${size}` : ''
    return `${BASE}/api/media/${mediaId}/thumbnail${qs}`
  },

  /** GET /api/media/:id/stream */
  streamUrl(mediaId) {
    return `${BASE}/api/media/${mediaId}/stream`
  },

  /** POST /api/scans */
  triggerScan(mode = 'quick', albumPath = '') {
    return request('/api/scans', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode, albumPath }),
    })
  },

  /** GET /api/scans/:id */
  getScanStatus(scanId) {
    return request(`/api/scans/${scanId}`)
  },

  /** GET /api/scans/active — returns active job or throws 404 */
  getActiveScan() {
    return request('/api/scans/active')
  },

  /** GET /api/media/:id/convert — direct URL for <img src>, returns JPEG */
  convertUrl(mediaId) {
    return `${BASE}/api/media/${mediaId}/convert`
  },

  /** POST /api/media/:id/transcode — returns { jobId } */
  triggerTranscode(mediaId) {
    return request(`/api/media/${mediaId}/transcode`, { method: 'POST' })
  },

  /** GET /api/transcode-jobs/:jobId */
  getTranscodeStatus(jobId) {
    return request(`/api/transcode-jobs/${jobId}`)
  },

  /** GET /api/media/:id/transcode/stream — direct URL for <video src> */
  transcodeStreamUrl(mediaId) {
    return `${BASE}/api/media/${mediaId}/transcode/stream`
  },

  /** GET /api/health */
  getHealth() {
    return request('/api/health')
  },

  /** GET /api/map/clusters?zoom=Z&bbox=W,S,E,N&album_id=X */
  getMapClusters(zoom, bbox, albumId) {
    const p = new URLSearchParams({ zoom: String(zoom) })
    if (bbox) p.set('bbox', `${bbox.west},${bbox.south},${bbox.east},${bbox.north}`)
    if (albumId != null) p.set('album_id', String(albumId))
    return request(`/api/map/clusters?${p}`)
  },

  /** GET /api/albums/:id/gps-count */
  getAlbumGPSCount(albumId) {
    return request(`/api/albums/${albumId}/gps-count`)
  },

  // --- AI persons / clusters ---

  /** GET /api/ai/clusters */
  getAIClusters() {
    return request('/api/ai/clusters')
  },

  /** POST /api/ai/persons — body: {clusterId, name} */
  createPerson(clusterId, name) {
    return request('/api/ai/persons', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ clusterId, name }),
    })
  },

  /** PUT /api/ai/persons/:id — body: {name} */
  renamePerson(personId, name) {
    return request(`/api/ai/persons/${personId}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    })
  },

  /** DELETE /api/ai/persons/:id */
  deletePerson(personId) {
    return request(`/api/ai/persons/${personId}`, { method: 'DELETE' })
  },

  /** POST /api/ai/persons/:id/merge/:otherId */
  mergePersons(personId, otherId) {
    return request(`/api/ai/persons/${personId}/merge/${otherId}`, { method: 'POST' })
  },

  /** DELETE /api/ai/clusters/:clusterId/faces/:faceId */
  removeFaceFromCluster(clusterId, faceId) {
    return request(`/api/ai/clusters/${clusterId}/faces/${faceId}`, { method: 'DELETE' })
  },

  /** DELETE /api/ai/faces/:faceId/person — unassign face from confirmed person */
  unassignFace(faceId) {
    return request(`/api/ai/faces/${faceId}/person`, { method: 'DELETE' })
  },

  /** GET /api/ai/persons */
  getPersons() {
    return request('/api/ai/persons')
  },

  /** GET /api/ai/persons/:id/media */
  getPersonMedia(personId, page = 1, pageSize = 50) {
    return request(`/api/ai/persons/${personId}/media${pageParams(page, pageSize)}`)
  },

  /** GET /api/ai/counts */
  getAICounts() {
    return request('/api/ai/counts')
  },

  /** POST /api/albums/by-media-ids */
  getAlbumsByMediaIDs(ids) {
    return request('/api/albums/by-media-ids', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ids }),
    })
  },
}
