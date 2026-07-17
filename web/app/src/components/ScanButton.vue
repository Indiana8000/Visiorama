<template>
  <div class="scan-btn-wrap">
    <span v-if="statusMsg" class="scan-status">{{ statusMsg }}</span>
    <button
      class="scan-btn scan-btn--reanalyze"
      :disabled="anyActive"
      title="Re-analyze AI for current album"
      @click="handleReanalyze"
    >Re-analyze</button>
    <button
      class="scan-btn"
      :disabled="anyActive"
      @click="handleScan('quick')"
    >Quick Scan</button>
    <button
      class="scan-btn scan-btn--full"
      :disabled="anyActive"
      @click="handleScan('full')"
      title="Re-index entire library"
    >Full</button>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api } from '../api/client.js'

const props = defineProps({
  albumPath: { type: String, default: '' },
})
const emit = defineEmits(['done'])

const POLL_INTERVAL_MS = 2000
const job = ref(null)
const pollTimer = ref(null)
const errorMsg = ref(null)
const reanalyzing = ref(false)
const reanalyzeQueued = ref(false)
const elapsedSec = ref(0)
let elapsedTimer = null
let scanStartMs = 0

const status = computed(() => job.value?.status ?? 'idle')
const isQueued = computed(() => status.value === 'queued')
const isRunning = computed(() => status.value === 'running')
const isDone = computed(() => status.value === 'success')
const isFailed = computed(() => status.value === 'failed')
const anyActive = computed(() => reanalyzing.value || isRunning.value || isQueued.value)

function startElapsed() {
  stopElapsed()
  scanStartMs = Date.now()
  elapsedSec.value = 0
  elapsedTimer = setInterval(() => {
    elapsedSec.value = Math.floor((Date.now() - scanStartMs) / 1000)
  }, 1000)
}
function stopElapsed() {
  if (elapsedTimer) { clearInterval(elapsedTimer); elapsedTimer = null }
  elapsedSec.value = 0
}
function fmtElapsed(s) {
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}

const statusMsg = computed(() => {
  if (errorMsg.value) return errorMsg.value
  if (reanalyzing.value) return 'Queuing…'
  if (reanalyzeQueued.value) return '✓ Re-analyze queued'
  if (!job.value) return null
  const mode = job.value.mode === 'full' ? 'full' : 'quick'
  const t = elapsedSec.value > 0 ? ` (${fmtElapsed(elapsedSec.value)})` : ''
  if (isRunning.value) {
    if (job.value.mode === 'quick') {
      const checked = job.value.scannedFiles
      return checked > 0
        ? `[quick] ${checked} album${checked === 1 ? '' : 's'} checked${t}`
        : `[quick] Scanning…${t}`
    }
    return `[${mode}] Indexed ${job.value.indexedFiles} / scanned ${job.value.scannedFiles}${t}`
  }
  if (isDone.value) {
    const fb = job.value.fallbackToFull ? ' (fell back to full)' : ''
    return `✓ ${job.value.indexedFiles} new, ${job.value.scannedFiles} scanned${fb}`
  }
  if (isFailed.value) return `✗ Errors: ${job.value.errorCount}`
  return null
})

function stopPolling() {
  if (pollTimer.value) {
    clearInterval(pollTimer.value)
    pollTimer.value = null
  }
}

async function poll(scanId) {
  try {
    const updated = await api.getScanStatus(scanId)
    job.value = updated
    if (updated.status === 'running' && elapsedTimer === null) startElapsed()
    if (updated.status === 'success' || updated.status === 'failed') {
      stopPolling()
      stopElapsed()
      emit('done', updated)
    }
  } catch (e) {
    stopPolling()
    stopElapsed()
    errorMsg.value = e.message
  }
}

onMounted(async () => {
  try {
    const active = await api.getActiveScan()
    job.value = active
    if (active.status === 'running') startElapsed()
    stopPolling()
    pollTimer.value = setInterval(() => poll(active.id), POLL_INTERVAL_MS)
  } catch {
    // no active scan — stay idle
  }
})

onUnmounted(() => {
  stopPolling()
  stopElapsed()
})

async function handleReanalyze() {
  errorMsg.value = null
  reanalyzeQueued.value = false
  reanalyzing.value = true
  try {
    await api.reanalyzeAlbum(props.albumPath)
    reanalyzeQueued.value = true
  } catch (e) {
    errorMsg.value = 'Re-analyze failed: ' + e.message
  } finally {
    reanalyzing.value = false
  }
}

async function handleScan(mode) {
  errorMsg.value = null
  reanalyzeQueued.value = false
  try {
    const newJob = await api.triggerScan(mode, props.albumPath)
    job.value = newJob
    startElapsed()
    stopPolling()
    pollTimer.value = setInterval(() => poll(newJob.id), POLL_INTERVAL_MS)
  } catch (e) {
    if (e.code === 'SCAN_ALREADY_RUNNING') {
      errorMsg.value = 'Scan already running'
    } else {
      errorMsg.value = e.message
    }
  }
}
</script>

<style scoped>
.scan-btn-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
}
.scan-btn {
  padding: 5px 14px;
  background: var(--bg3);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--muted);
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
}
.scan-btn:hover:not(:disabled) {
  background: var(--accent);
  border-color: var(--accent);
  color: #fff;
}
.scan-btn--full {
  padding: 5px 10px;
}
.scan-btn--full:hover:not(:disabled) {
  background: #7c3aed;
  border-color: #7c3aed;
  color: #fff;
}
.scan-btn--reanalyze {
  padding: 5px 10px;
}
.scan-btn--reanalyze:hover:not(:disabled) {
  background: #0891b2;
  border-color: #0891b2;
  color: #fff;
}
.scan-btn:disabled { opacity: 0.4; cursor: default; }

.scan-status { font-size: 12px; color: var(--muted); max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
