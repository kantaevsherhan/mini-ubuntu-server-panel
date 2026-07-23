<script setup lang="ts">
import Message from 'primevue/message'
import NotificationSettings from '../components/NotificationSettings.vue'
import { useI18n } from '../services/i18n'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const { t } = useI18n()
</script>

<template>
  <section>
    <div class="mb-4">
      <h1 class="m-0 text-2xl font-semibold">{{ t.notifications }}</h1>
      <p class="muted mb-0 mt-2">{{ t.notificationsHint }}</p>
    </div>
    <Message v-if="auth.role !== 'admin'" severity="info" :closable="false" class="mb-4">
      {{ t.notificationHistoryHint }}
    </Message>
    <NotificationSettings :read-only="auth.role !== 'admin'" />
  </section>
</template>
