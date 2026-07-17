import { createRouter, createWebHistory } from 'vue-router'
import AlbumView from '../views/AlbumView.vue'
import LightboxView from '../views/LightboxView.vue'
import MapView from '../views/MapView.vue'
import PersonsView from '../views/PersonsView.vue'

const routes = [
  {
    path: '/',
    name: 'root',
    component: AlbumView,
  },
  {
    path: '/album/:id',
    name: 'album',
    component: AlbumView,
    props: true,
  },
  {
    path: '/media/:id',
    name: 'media',
    component: LightboxView,
    props: true,
  },
  {
    path: '/map',
    name: 'map',
    component: MapView,
    props: route => ({ albumId: route.query.album_id || null }),
  },
  {
    path: '/persons',
    name: 'persons',
    component: PersonsView,
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
