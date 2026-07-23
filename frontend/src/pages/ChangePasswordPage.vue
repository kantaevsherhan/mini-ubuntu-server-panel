<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import FloatLabel from 'primevue/floatlabel'
import Fluid from 'primevue/fluid'
import Message from 'primevue/message'
import Password from 'primevue/password'
import { useAuthStore } from '../stores/auth'
import { useI18n } from '../services/i18n'

const currentPassword = ref('')
const newPassword = ref('')
const confirmation = ref('')
const loading = ref(false)
const error = ref('')
const auth = useAuthStore()
const router = useRouter()
const { t } = useI18n()

async function submit() {
  error.value = ''
  if (newPassword.value.length < 12 || newPassword.value !== confirmation.value) {
    error.value = t.value.passwordRequirements
    return
  }
  loading.value = true
  try {
    await auth.changePassword(currentPassword.value, newPassword.value)
    router.push('/')
  } catch {
    error.value = t.value.passwordChangeFailed
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="grid h-full place-items-center">
    <Card class="mx-4 w-full max-w-[440px]">
      <template #title>{{ t.changeTemporaryPassword }}</template>
      <template #subtitle>{{ t.changeTemporaryPasswordHint }}</template>
      <template #content>
        <Fluid>
          <form class="space-y-6 pt-3" @submit.prevent="submit">
            <Message v-if="error" severity="error">{{ error }}</Message>
            <FloatLabel variant="on">
              <Password
                id="current-password"
                v-model="currentPassword"
                :feedback="false"
                toggle-mask
              />
              <label for="current-password">{{ t.currentPassword }}</label>
            </FloatLabel>
            <FloatLabel variant="on">
              <Password id="new-password" v-model="newPassword" toggle-mask />
              <label for="new-password">{{ t.newPassword }}</label>
            </FloatLabel>
            <FloatLabel variant="on">
              <Password
                id="confirm-password"
                v-model="confirmation"
                :feedback="false"
                toggle-mask
              />
              <label for="confirm-password">{{ t.confirmPassword }}</label>
            </FloatLabel>
            <Button type="submit" :label="t.changePassword" icon="pi pi-key" :loading="loading" />
          </form>
        </Fluid>
      </template>
    </Card>
  </div>
</template>
