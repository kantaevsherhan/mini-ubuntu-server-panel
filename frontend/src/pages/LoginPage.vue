<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import FloatLabel from 'primevue/floatlabel'
import Fluid from 'primevue/fluid'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import { useAuthStore } from '../stores/auth'
import { useI18n } from '../services/i18n'

const username = ref('admin')
const password = ref('')
const loading = ref(false)
const error = ref('')
const router = useRouter()
const auth = useAuthStore()
const { t } = useI18n()

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await auth.login(username.value, password.value)
    router.push(auth.mustChangePassword ? '/change-password' : '/')
  } catch {
    error.value = t.value.invalidCredentials
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-shell">
    <Card class="login-card mx-4 w-full max-w-[400px]">
      <template #title>
        <div class="text-center">
          <span class="login-mark"><i class="pi pi-server" /></span>
          <h1 class="mb-1 mt-4 text-xl font-semibold tracking-tight">Mini Ubuntu Server Panel</h1>
          <p class="muted m-0 text-sm font-normal">{{ t.signInSubtitle }}</p>
        </div>
      </template>
      <template #content>
        <Fluid>
          <form class="space-y-6 pt-3" @submit.prevent="submit">
            <Message v-if="error" severity="error">{{ error }}</Message>
            <FloatLabel variant="on">
              <InputText id="username" v-model="username" autocomplete="username" />
              <label for="username">{{ t.username }}</label>
            </FloatLabel>
            <FloatLabel variant="on">
              <Password
                id="password"
                v-model="password"
                :feedback="false"
                toggle-mask
                autocomplete="current-password"
              />
              <label for="password">{{ t.password }}</label>
            </FloatLabel>
            <Button type="submit" :label="t.login" icon="pi pi-sign-in" :loading="loading" />
          </form>
        </Fluid>
      </template>
    </Card>
  </div>
</template>

<style scoped>
.login-shell {
  display: grid;
  height: 100%;
  place-items: center;
  background:
    radial-gradient(700px 360px at 50% 0%, var(--app-glow), transparent 70%) no-repeat,
    var(--app-background);
}

.login-card {
  box-shadow: var(--app-shadow-raised) !important;
}

.login-mark {
  display: inline-grid;
  place-items: center;
  width: 48px;
  height: 48px;
  border-radius: 14px;
  font-size: 1.3rem;
  color: var(--p-primary-contrast-color);
  background: linear-gradient(140deg, var(--p-primary-400), var(--p-primary-700));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.25),
    0 8px 24px color-mix(in srgb, var(--p-primary-500) 35%, transparent);
}
</style>
