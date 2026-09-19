<script setup lang="ts">
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import MonitorSettings from '../components/MonitorSettings.vue'
import NotificationSettings from '../components/NotificationSettings.vue'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const { t } = useI18n()
</script>

<template>
  <section class="space-y-4">
    <header class="page-header !mb-0">
      <div>
        <h1 class="page-title">{{ t.notifications }}</h1>
        <p class="page-subtitle">{{ t.notificationsHint }}</p>
      </div>
      <Button
        v-if="auth.role === 'admin'"
        :label="t.telegramSetup"
        icon="pi pi-send"
        severity="secondary"
        outlined
        @click="router.push({ path: '/settings', query: { tab: 'telegram' } })"
      />
    </header>
    <Message v-if="auth.role !== 'admin'" severity="info" :closable="false">
      {{ t.notificationHistoryHint }}
    </Message>
    <MonitorSettings v-if="auth.role === 'admin'" />
    <NotificationSettings :read-only="auth.role !== 'admin'" />
  </section>
</template>
