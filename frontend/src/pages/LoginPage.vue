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
    error.value = 'Неверное имя пользователя или пароль'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="grid h-full place-items-center">
    <Card class="mx-4 w-full max-w-[400px]">
      <template #title>
        <div class="text-center">
          <i class="pi pi-server text-4xl text-primary" />
          <h1 class="mt-3 text-xl font-semibold">Mini Ubuntu Server Panel</h1>
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
