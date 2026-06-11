<template>
  <div class="scan-btn-wrap">
    <button
      class="scan-btn"
      :class="{ 'scan-btn--running': isRunning, 'scan-btn--done': isDone, 'scan-btn--failed': isFailed }"
      :disabled="isRunning || isQueued"
      @click="handleScan('quick')"
    >
      <span v-if="isIdle">Quick Scan</span>
      <span v-else-if="isQueued">Queued…</span>
      <span v-else-if="isRunning">Scanning…</span>
      <span v-else-if="isDone">&#10003; Done</span>
      <span v-else-if="isFailed">&#10007; Failed</span>
    </button>
    <button
      class="scan-btn scan-btn--full"
      :disabled="isRunning || isQueued"
      @click="handleScan('full')"
      title="Re-index entire library"
    >
      Full
    </button>
    <span v-if="statusMsg" class="scan-status">{{ statusMsg }}</span>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api/client.js'

const emit = defineEmits(['done'])

const POLL_INTERVAL_MS = 2000
const job = ref(null)
const pollTimer = ref(null)
const errorMsg = ref(null)

const status = computed(() => job.value?.status ?? 'idle')
const isIdle = computed(() => status.value === 'idle')
const isQueued = computed(() => status.value === 'queued')
const isRunning = computed(() => status.value === 'running')
const isDone = computed(() => status.value === 'success')
const isFailed = computed(() => status.value === 'failed')

const statusMsg = computed(() => {
  if (errorMsg.value) return errorMsg.value
  if (!job.value) return null
  const mode = job.value.mode === 'full' ? 'full' : 'quick'
  if (isRunning.value) {
    if (job.value.mode === 'quick') {
      const checked = job.value.scannedFiles
      return checked > 0
        ? `[quick] ${checked} album${checked === 1 ? '' : 's'} checked…`
        : '[quick] Checking…'
    }
    return `[${mode}] Indexed ${job.value.indexedFiles} / scanned ${job.value.scannedFiles}`
  }
  if (isDone.value) {
    const fb = job.value.fallbackToFull ? ' (fell back to full)' : ''
    return `[${mode}] ${job.value.indexedFiles} new, ${job.value.scannedFiles} scanned${fb}`
  }
  if (isFailed.value) return `Errors: ${job.value.errorCount}`
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
    if (updated.status === 'success' || updated.status === 'failed') {
      stopPolling()
      emit('done', updated)
      setTimeout(() => { job.value = null }, 5000)
    }
  } catch (e) {
    stopPolling()
    errorMsg.value = e.message
  }
}

onMounted(async () => {
  try {
    const active = await api.getActiveScan()
    job.value = active
    stopPolling()
    pollTimer.value = setInterval(() => poll(active.id), POLL_INTERVAL_MS)
  } catch {
    // no active scan — stay idle
  }
})

async function handleScan(mode) {
  errorMsg.value = null
  try {
    const newJob = await api.triggerScan(mode)
    job.value = newJob
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
  color: var(--text);
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
  opacity: 0.7;
}
.scan-btn--full:hover:not(:disabled) {
  opacity: 1;
  background: #7c3aed;
  border-color: #7c3aed;
  color: #fff;
}
.scan-btn:disabled { opacity: 0.4; cursor: default; }
.scan-btn--running { border-color: var(--accent); color: var(--accent); }
.scan-btn--done { border-color: var(--success); color: var(--success); }
.scan-btn--failed { border-color: var(--danger); color: var(--danger); }

.scan-status { font-size: 12px; color: var(--muted); max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
