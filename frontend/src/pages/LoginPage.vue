<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useRouter } from 'vue-router'
import { isAxiosError } from 'axios'
import Button from 'primevue/button'
import Card from 'primevue/card'
import FloatLabel from 'primevue/floatlabel'
import Fluid from 'primevue/fluid'
import InputOtp from 'primevue/inputotp'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import { useAuthStore } from '../stores/auth'
import { useI18n } from '../services/i18n'

const username = ref('admin')
const password = ref('')
const code = ref('')
const recoveryCode = ref('')
const useRecovery = ref(false)
const loading = ref(false)
const error = ref('')
const router = useRouter()
const auth = useAuthStore()
const { t } = useI18n()
const secondStep = computed(() => Boolean(auth.mfaToken))
const codeReady = computed(() =>
  useRecovery.value ? recoveryCode.value.trim().length >= 10 : code.value.length === 6,
)

function describe(failure: unknown, fallback: string) {
  if (!isAxiosError(failure) || !failure.response) return t.value.networkError
  const reason = failure.response.data?.error
  if (failure.response.status === 429) return t.value.tooManyAttempts
  if (reason === 'mfa_challenge_invalid') return t.value.challengeExpired
  if (reason === 'mfa_code_invalid') return t.value.invalidCode
  return fallback
}

function finish() {
  router.push(auth.mustChangePassword ? '/change-password' : '/')
}

async function submit() {
  loading.value = true
  error.value = ''
  try {
    if (await auth.login(username.value, password.value)) {
      password.value = ''
      await nextTick()
      // Do not steal focus if the user already started typing into the code boxes.
      if (!document.activeElement?.closest('.login-otp')) {
        document.querySelector<HTMLInputElement>('.login-otp input')?.focus()
      }
      return
    }
    finish()
  } catch (failure) {
    error.value = describe(failure, t.value.invalidCredentials)
  } finally {
    loading.value = false
  }
}

async function verify() {
  if (!codeReady.value || loading.value) return
  loading.value = true
  error.value = ''
  try {
    await auth.verifySecondFactor(useRecovery.value ? recoveryCode.value : code.value)
    finish()
  } catch (failure) {
    error.value = describe(failure, t.value.invalidCode)
    code.value = ''
    if (error.value === t.value.challengeExpired) back()
  } finally {
    loading.value = false
  }
}

function back() {
  auth.cancelSecondFactor()
  code.value = ''
  recoveryCode.value = ''
  useRecovery.value = false
}

function onCodeInput(value: unknown) {
  // Submit as soon as the sixth digit arrives, like most authenticator flows.
  if (typeof value === 'string' && value.length === 6) void verify()
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
        <Fluid v-if="!secondStep">
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
        <form v-else class="space-y-5 pt-3" @submit.prevent="verify">
          <div class="text-center">
            <span class="stat-icon mx-auto mb-3 !h-10 !w-10"><i class="pi pi-shield" /></span>
            <h2 class="m-0 text-base font-semibold">{{ t.loginSecondFactorTitle }}</h2>
            <p class="muted mb-0 mt-1 text-sm">
              {{ useRecovery ? t.twoFactorConfirmHint : t.loginSecondFactorHint }}
            </p>
          </div>
          <Message v-if="error" severity="error">{{ error }}</Message>
          <div v-if="!useRecovery" class="login-otp flex justify-center">
            <InputOtp
              v-model="code"
              :length="6"
              integer-only
              :aria-label="t.twoFactorCode"
              @update:model-value="onCodeInput"
            />
          </div>
          <Fluid v-else>
            <FloatLabel variant="on">
              <InputText
                id="recovery-code"
                v-model="recoveryCode"
                autocomplete="one-time-code"
                class="font-mono"
              />
              <label for="recovery-code">{{ t.recoveryCode }}</label>
            </FloatLabel>
          </Fluid>
          <Button
            type="submit"
            :label="t.verify"
            icon="pi pi-check"
            :loading="loading"
            :disabled="!codeReady"
            fluid
          />
          <div class="flex items-center justify-between gap-2">
            <Button :label="t.back" icon="pi pi-arrow-left" text size="small" @click="back" />
            <Button
              :label="useRecovery ? t.useAuthenticatorCode : t.useRecoveryCode"
              text
              size="small"
              @click="((useRecovery = !useRecovery), (error = ''))"
            />
          </div>
        </form>
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
