<template>
  <div class="persons-view">

    <!-- Cluster review section -->
    <section v-if="clusters.length > 0" class="section">
      <h2 class="section-title">
        Review Clusters
        <span class="badge">{{ clusters.length }}</span>
      </h2>
      <p class="hint">Name these groups of faces to add them to your persons gallery.</p>

      <div class="clusters">
        <div v-for="cluster in clusters" :key="cluster.clusterId" class="cluster-card">
          <div class="cluster-faces">
            <div
              v-for="face in cluster.faces.slice(0, 6)"
              :key="face.faceId"
              class="face-wrap"
            >
              <img
                v-if="face.cropPath"
                :src="face.cropPath"
                class="face-crop"
                loading="lazy"
              />
              <div v-else class="face-placeholder">?</div>
              <button
                class="face-remove"
                title="Remove from cluster"
                @click="removeFace(cluster, face)"
              >✕</button>
            </div>
            <div v-if="cluster.faces.length > 6" class="face-more">
              +{{ cluster.faces.length - 6 }}
            </div>
          </div>

          <div class="cluster-footer">
            <input
              v-model="clusterNames[cluster.clusterId]"
              class="name-input"
              :placeholder="`Name (${cluster.faces.length} face${cluster.faces.length !== 1 ? 's' : ''})`"
              @keydown.enter="namePerson(cluster)"
            />
            <button
              class="btn-primary"
              :disabled="!clusterNames[cluster.clusterId]?.trim()"
              @click="namePerson(cluster)"
            >Save</button>
          </div>
        </div>
      </div>
    </section>

    <!-- Persons gallery -->
    <section class="section">
      <h2 class="section-title">Persons</h2>

      <div v-if="loading" class="state-msg">Loading…</div>
      <div v-else-if="persons.length === 0 && clusters.length === 0" class="empty-state">
        <p class="empty-state__msg">No persons yet. Run an AI analysis to detect faces.</p>
      </div>

      <div v-else-if="selectedPerson" class="person-media">
        <div class="person-media-header">
          <button class="btn-back" @click="deselectPerson">← Back</button>
          <span class="person-media-name">{{ selectedPerson.name }}</span>
          <span class="person-media-count">{{ selectedPerson.mediaCount }} photos</span>
          <div class="person-actions">
            <button class="btn-sm" @click="startRename(selectedPerson)">Rename</button>
            <button class="btn-sm btn-danger" @click="confirmDelete(selectedPerson)">Delete</button>
          </div>
        </div>

        <div v-if="personMedia.length > 0" class="grid grid--media">
          <MediaTile
            v-for="item in personMedia"
            :key="item.id"
            :media="item"
            :extra-query="{ from: 'persons', personId: selectedPerson.id }"
          />
        </div>
        <div v-if="mediaPage.totalPages > 1" class="pagination">
          <button class="page-btn" :disabled="!mediaPage.hasPrev" @click="loadPersonMedia(mediaPage.page - 1)">← Prev</button>
          <span class="page-info">{{ mediaPage.page }} / {{ mediaPage.totalPages }}</span>
          <button class="page-btn" :disabled="!mediaPage.hasNext" @click="loadPersonMedia(mediaPage.page + 1)">Next →</button>
        </div>
      </div>

      <div v-else class="grid grid--persons">
        <div
          v-for="person in persons"
          :key="person.id"
          class="person-tile"
          @click="selectPerson(person)"
        >
          <div class="person-tile__cover">
            <img
              v-if="person.coverCrop"
              :src="person.coverCrop"
              class="person-tile__img"
              loading="lazy"
            />
            <div v-else class="person-tile__placeholder">👤</div>
          </div>
          <div class="person-tile__info">
            <span class="person-tile__name" :title="person.name">{{ person.name }}</span>
            <span class="person-tile__count">{{ person.mediaCount }} photos</span>
          </div>
        </div>
      </div>
    </section>

    <!-- Rename modal -->
    <div v-if="renameTarget" class="modal-backdrop" @click.self="renameTarget = null">
      <div class="modal">
        <h3>Rename Person</h3>
        <input v-model="renameValue" class="name-input" autofocus @keydown.enter="doRename" />
        <div class="modal-actions">
          <button class="btn-primary" @click="doRename">Save</button>
          <button class="btn-sm" @click="renameTarget = null">Cancel</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client.js'
import MediaTile from '../components/MediaTile.vue'

const props = defineProps({
  personId: { type: String, default: null },
})

const router = useRouter()

const clusters = ref([])
const clusterNames = ref({})
const persons = ref([])
const loading = ref(false)
const selectedPerson = ref(null)
const personMedia = ref([])
const mediaPage = ref({ page: 1, totalPages: 1, hasPrev: false, hasNext: false })
const renameTarget = ref(null)
const renameValue = ref('')

async function load() {
  loading.value = true
  try {
    const [cls, ps] = await Promise.all([api.getAIClusters(), api.getPersons()])
    clusters.value = cls
    persons.value = ps
    // init name inputs
    const names = {}
    cls.forEach(c => { names[c.clusterId] = '' })
    clusterNames.value = names
  } catch (e) {
    console.error('persons load failed', e)
  } finally {
    loading.value = false
  }
}

async function namePerson(cluster) {
  const name = clusterNames.value[cluster.clusterId]?.trim()
  if (!name) return
  try {
    await api.createPerson(cluster.clusterId, name)
    await load()
  } catch (e) {
    alert('Failed to save: ' + e.message)
  }
}

async function removeFace(cluster, face) {
  try {
    await api.removeFaceFromCluster(cluster.clusterId, face.faceId)
    cluster.faces = cluster.faces.filter(f => f.faceId !== face.faceId)
    if (cluster.faces.length === 0) {
      clusters.value = clusters.value.filter(c => c.clusterId !== cluster.clusterId)
    }
  } catch (e) {
    alert('Failed to remove face: ' + e.message)
  }
}

async function selectPerson(person) {
  selectedPerson.value = person
  router.replace({ name: 'person', params: { personId: person.id } })
  await loadPersonMedia(1)
}

function deselectPerson() {
  selectedPerson.value = null
  router.replace({ name: 'persons' })
}

async function loadPersonMedia(page) {
  try {
    const res = await api.getPersonMedia(selectedPerson.value.id, page)
    personMedia.value = res.media
    mediaPage.value = res.page
  } catch (e) {
    console.error('person media load failed', e)
  }
}

function startRename(person) {
  renameTarget.value = person
  renameValue.value = person.name
}

async function doRename() {
  if (!renameTarget.value || !renameValue.value.trim()) return
  try {
    await api.renamePerson(renameTarget.value.id, renameValue.value.trim())
    renameTarget.value.name = renameValue.value.trim()
    if (selectedPerson.value?.id === renameTarget.value.id) {
      selectedPerson.value.name = renameValue.value.trim()
    }
    renameTarget.value = null
    await load()
  } catch (e) {
    alert('Rename failed: ' + e.message)
  }
}

async function confirmDelete(person) {
  if (!confirm(`Delete "${person.name}"? This cannot be undone.`)) return
  try {
    await api.deletePerson(person.id)
    selectedPerson.value = null
    await load()
  } catch (e) {
    alert('Delete failed: ' + e.message)
  }
}

async function openPersonById(id) {
  const numId = parseInt(id, 10)
  if (!numId) return
  // persons may not be loaded yet — load first if needed
  let p = persons.value.find(x => x.id === numId)
  if (!p) {
    await load()
    p = persons.value.find(x => x.id === numId)
  }
  if (p) {
    selectedPerson.value = p
    await loadPersonMedia(1)
  }
}

onMounted(async () => {
  if (props.personId) {
    await openPersonById(props.personId)
  } else {
    await load()
  }
})

watch(() => props.personId, (id) => {
  if (!id) {
    selectedPerson.value = null
  } else {
    openPersonById(id)
  }
})
</script>

<style scoped>
.persons-view { padding-bottom: 40px; }

.section { margin-bottom: 40px; }
.section-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.badge {
  background: var(--accent);
  color: #1e1e2e;
  border-radius: 10px;
  padding: 1px 7px;
  font-size: 11px;
  font-weight: 700;
}
.hint { font-size: 13px; color: var(--muted); margin-bottom: 16px; }

/* Cluster cards */
.clusters { display: flex; flex-direction: column; gap: 16px; }
.cluster-card {
  background: var(--bg2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 12px;
}
.cluster-faces {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}
.face-wrap {
  position: relative;
  width: 72px;
  height: 72px;
}
.face-crop {
  width: 72px;
  height: 72px;
  object-fit: cover;
  border-radius: 50%;
  border: 2px solid var(--border);
}
.face-placeholder {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  background: var(--bg3);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
}
.face-remove {
  position: absolute;
  top: -4px;
  right: -4px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  border: none;
  background: #f38ba8;
  color: #1e1e2e;
  font-size: 10px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  line-height: 1;
}
.face-more {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  background: var(--bg3);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  color: var(--muted);
}
.cluster-footer { display: flex; gap: 8px; align-items: center; }
.name-input {
  flex: 1;
  background: var(--bg3);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--text);
  padding: 6px 10px;
  font-size: 13px;
}
.name-input:focus { outline: none; border-color: var(--accent); }

/* Persons grid */
.grid--persons {
  display: grid;
  gap: var(--gap);
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
}
.grid--media {
  display: grid;
  gap: var(--gap);
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
}

.person-tile {
  display: flex;
  flex-direction: column;
  background: var(--bg2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
  cursor: pointer;
  transition: transform 0.15s, border-color 0.15s;
}
.person-tile:hover { transform: scale(1.03); border-color: var(--accent); }
.person-tile__cover {
  aspect-ratio: 1;
  background: var(--bg3);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}
.person-tile__img { width: 100%; height: 100%; object-fit: cover; }
.person-tile__placeholder { font-size: 48px; }
.person-tile__info {
  padding: 8px;
  background: var(--bg3);
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.person-tile__name {
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.person-tile__count { font-size: 11px; color: var(--muted); }

/* Person media detail */
.person-media-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.person-media-name { font-size: 18px; font-weight: 600; }
.person-media-count { font-size: 13px; color: var(--muted); }
.person-actions { display: flex; gap: 8px; margin-left: auto; }

/* Buttons */
.btn-primary {
  background: var(--accent);
  color: #1e1e2e;
  border: none;
  border-radius: var(--radius);
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.btn-primary:disabled { opacity: 0.4; cursor: default; }
.btn-back {
  background: var(--bg2);
  border: 1px solid var(--border);
  color: var(--text);
  border-radius: var(--radius);
  padding: 5px 12px;
  font-size: 13px;
  cursor: pointer;
}
.btn-back:hover { background: var(--bg3); }
.btn-sm {
  background: var(--bg2);
  border: 1px solid var(--border);
  color: var(--text);
  border-radius: var(--radius);
  padding: 4px 10px;
  font-size: 12px;
  cursor: pointer;
}
.btn-sm:hover { background: var(--bg3); }
.btn-danger { border-color: #f38ba8; color: #f38ba8; }
.btn-danger:hover { background: rgba(243,139,168,0.1); }

/* Pagination */
.pagination {
  display: flex;
  align-items: center;
  gap: 12px;
  justify-content: center;
  margin-top: 24px;
}
.page-btn {
  padding: 5px 14px;
  background: var(--bg2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--text);
  cursor: pointer;
  font-size: 13px;
}
.page-btn:disabled { opacity: 0.4; cursor: default; }
.page-info { font-size: 13px; color: var(--muted); }

/* Modal */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}
.modal {
  background: var(--bg2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 24px;
  min-width: 280px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.modal h3 { margin: 0; font-size: 16px; }
.modal-actions { display: flex; gap: 8px; justify-content: flex-end; }

/* States */
.state-msg { padding: 40px; text-align: center; color: var(--muted); }
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 60px 20px;
}
.empty-state__msg { color: var(--muted); font-size: 15px; }

@media (max-width: 480px) {
  .grid--persons { grid-template-columns: repeat(2, 1fr); }
  .grid--media { grid-template-columns: repeat(2, 1fr); }
}
</style>
