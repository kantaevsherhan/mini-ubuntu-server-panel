<script setup lang="ts">
import { onMounted, ref } from 'vue'
import ProgressBar from 'primevue/progressbar'
import Skeleton from 'primevue/skeleton'
import api from '../services/api'
import { useI18n } from '../services/i18n'
const { t } = useI18n()
const data = ref<Record<string, any>>()
onMounted(async () => { data.value = (await api.get('/dashboard')).data })
const cards = [{label:'CPU',value:34,icon:'pi pi-microchip'},{label:'RAM',value:62,icon:'pi pi-database'},{label:'Disk',value:48,icon:'pi pi-save'}]
</script>
<template><section><div class="flex items-center mb-5"><div><h1 class="m-0 text-2xl font-semibold">{{t.welcome}}</h1><p class="muted mt-1">{{data?.hostname||'ubuntu-server'}}</p></div><span class="flex-1"/><span class="text-sm text-green-500"><i class="pi pi-circle-fill text-[8px] mr-2"/>{{t.online}}</span></div><div class="grid grid-cols-3 gap-4"><article v-for="card in cards" :key="card.label" class="panel-card p-4"><div class="flex"><i :class="card.icon" class="text-primary"/><span class="ml-2 font-medium">{{card.label}}</span><b class="ml-auto">{{card.value}}%</b></div><ProgressBar :value="card.value" :show-value="false" class="h-2 mt-4"/></article></div><div class="grid grid-cols-2 gap-4 mt-4"><article class="panel-card p-5"><div class="muted text-sm">{{t.panelUsers}}</div><Skeleton v-if="!data" width="4rem" height="2rem" class="mt-3"/><div v-else class="text-3xl mt-2">{{data.panel_users}}</div></article><article class="panel-card p-5"><div class="muted text-sm">{{t.pending}}</div><div class="text-3xl mt-2">{{data?.pending_notifications??0}}</div></article></div></section></template>
