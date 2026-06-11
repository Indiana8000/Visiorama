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
  triggerScan(mode = 'quick') {
    return request('/api/scans', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode }),
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
}
