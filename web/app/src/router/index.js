import { createRouter, createWebHistory } from 'vue-router'
import AlbumView from '../views/AlbumView.vue'
import LightboxView from '../views/LightboxView.vue'

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
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
