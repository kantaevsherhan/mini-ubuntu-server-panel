<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { isAxiosError } from 'axios'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import FloatLabel from 'primevue/floatlabel'
import Fluid from 'primevue/fluid'
import InputOtp from 'primevue/inputotp'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'
import Skeleton from 'primevue/skeleton'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'

interface TwoFactorStatus {
  enabled: boolean
  enabled_at: string | null
  recovery_codes_remaining: number
}

type Mode = 'enable' | 'disable' | 'regenerate'
type Step = 'password' | 'scan' | 'confirm' | 'codes'

const { t, locale } = useI18n()
const toast = useToast()
const status = ref<TwoFactorStatus>()
const visible = ref(false)
const mode = ref<Mode>('enable')
const step = ref<Step>('password')
const password = ref('')
const code = ref('')
const secret = ref('')
const qrCode = ref('')
const recoveryCodes = ref<string[]>([])
const error = ref('')
const busy = ref(false)

const dialogTitle = computed(() => {
  if (step.value === 'codes') return t.value.recoveryCodes
  if (mode.value === 'disable') return t.value.disableTwoFactor
  if (mode.value === 'regenerate') return t.value.regenerateRecoveryCodes
  return t.value.enableTwoFactor
})

async function load() {
  status.value = (await api.get<TwoFactorStatus>('/auth/2fa')).data
}

function open(next: Mode) {
  mode.value = next
  step.value = next === 'enable' ? 'password' : 'confirm'
  password.value = ''
  code.value = ''
  secret.value = ''
  qrCode.value = ''
  recoveryCodes.value = []
  error.value = ''
  visible.value = true
}

function explain(failure: unknown) {
  const reason = isAxiosError(failure) ? failure.response?.data?.error : undefined
  if (reason === 'current_password_invalid') return t.value.invalidPassword
  if (reason === 'mfa_code_invalid') return t.value.invalidCode
  if (reason === 'mfa_setup_required') return t.value.challengeExpired
  return t.value.requestError
}

async function run(action: () => Promise<void>) {
  busy.value = true
  error.value = ''
  try {
    await action()
  } catch (failure) {
    error.value = explain(failure)
  } finally {
    busy.value = false
  }
}

function startSetup() {
  return run(async () => {
    const { data } = await api.post<{ secret: string; otpauth_uri: string }>(
      '/auth/2fa/setup',
      { password: password.value },
      { silent: true },
    )
    password.value = ''
    secret.value = data.secret
    // qrcode is only needed during enrollment, so it stays out of the main bundle.
    const QRCode = await import('qrcode')
    qrCode.value = await QRCode.toDataURL(data.otpauth_uri, { margin: 1, width: 200 })
    step.value = 'scan'
  })
}

function confirmEnrollment() {
  return run(async () => {
    const { data } = await api.post<{ recovery_codes: string[] }>(
      '/auth/2fa/enable',
      { code: code.value },
      { silent: true },
    )
    recoveryCodes.value = data.recovery_codes
    secret.value = ''
    qrCode.value = ''
    step.value = 'codes'
    toast.add({ severity: 'success', summary: t.value.twoFactorEnabledToast, life: 4000 })
    await load()
  })
}

function confirmSensitive() {
  return run(async () => {
    const body = { password: password.value, code: code.value.trim() }
    if (mode.value === 'disable') {
      await api.post('/auth/2fa/disable', body, { silent: true })
      visible.value = false
      toast.add({ severity: 'success', summary: t.value.twoFactorDisabledToast, life: 4000 })
    } else {
      const { data } = await api.post<{ recovery_codes: string[] }>(
        '/auth/2fa/recovery-codes',
        body,
        { silent: true },
      )
      recoveryCodes.value = data.recovery_codes
      step.value = 'codes'
    }
    password.value = ''
    await load()
  })
}

async function copyCodes() {
  await navigator.clipboard.writeText(recoveryCodes.value.join('\n'))
  toast.add({ severity: 'success', summary: t.value.copied, life: 2000 })
}

function downloadCodes() {
  const blob = new Blob([recoveryCodes.value.join('\n') + '\n'], { type: 'text/plain' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = 'mini-ubuntu-server-recovery-codes.txt'
  link.click()
  URL.revokeObjectURL(link.href)
}

function onHide() {
  // Never keep secrets or codes around after the dialog closes.
  password.value = ''
  code.value = ''
  secret.value = ''
  qrCode.value = ''
  recoveryCodes.value = []
}

onMounted(load)
</script>

<template>
  <div class="panel-card p-5">
    <div class="flex flex-wrap items-start gap-4">
      <span class="stat-icon !h-10 !w-10"><i class="pi pi-shield" /></span>
      <div class="min-w-0 flex-1">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="m-0 text-base font-semibold">{{ t.twoFactor }}</h3>
          <Skeleton v-if="!status" width="5rem" height="1.4rem" />
          <Tag
            v-else
            :severity="status.enabled ? 'success' : 'secondary'"
            :icon="status.enabled ? 'pi pi-check' : 'pi pi-minus'"
            :value="status.enabled ? t.twoFactorEnabled : t.twoFactorDisabled"
          />
        </div>
        <p class="muted mb-0 mt-1 text-sm">{{ t.twoFactorHint }}</p>
        <p v-if="status?.enabled" class="muted mb-0 mt-2 text-sm">
          <template v-if="status.enabled_at">{{
            formatDateTime(status.enabled_at, locale)
          }}</template>
          · {{ t.recoveryCodesRemaining }}:
          <strong
            :class="status.recovery_codes_remaining < 3 ? 'text-[var(--app-warn)]' : ''"
            class="tabular"
            >{{ status.recovery_codes_remaining }}</strong
          >
        </p>
      </div>
      <div v-if="status" class="flex flex-wrap gap-2">
        <template v-if="status.enabled">
          <Button
            :label="t.regenerateRecoveryCodes"
            icon="pi pi-refresh"
            severity="secondary"
            outlined
            size="small"
            @click="open('regenerate')"
          />
          <Button
            :label="t.disableTwoFactor"
            icon="pi pi-times"
            severity="danger"
            outlined
            size="small"
            @click="open('disable')"
          />
        </template>
        <Button
          v-else
          :label="t.enableTwoFactor"
          icon="pi pi-lock"
          size="small"
          @click="open('enable')"
        />
      </div>
    </div>
  </div>

  <Dialog
    v-model:visible="visible"
    modal
    :header="dialogTitle"
    :closable="step !== 'codes'"
    class="w-[min(460px,calc(100vw-24px))]"
    @hide="onHide"
  >
    <Message v-if="error" severity="error" class="mb-4">{{ error }}</Message>

    <form v-if="step === 'password'" class="space-y-5" @submit.prevent="startSetup">
      <p class="muted m-0 text-sm">{{ t.twoFactorStepPassword }}</p>
      <Fluid>
        <FloatLabel variant="on">
          <Password
            id="mfa-password"
            v-model="password"
            :feedback="false"
            toggle-mask
            autocomplete="current-password"
          />
          <label for="mfa-password">{{ t.currentPassword }}</label>
        </FloatLabel>
      </Fluid>
      <div class="flex justify-end gap-2">
        <Button :label="t.cancel" severity="secondary" text @click="visible = false" />
        <Button
          type="submit"
          :label="t.continueAction"
          :loading="busy"
          :disabled="!password"
          icon="pi pi-arrow-right"
          icon-pos="right"
        />
      </div>
    </form>

    <form v-else-if="step === 'scan'" class="space-y-5" @submit.prevent="confirmEnrollment">
      <p class="muted m-0 text-sm">{{ t.twoFactorStepScan }}</p>
      <div class="flex flex-col items-center gap-3">
        <img
          v-if="qrCode"
          :src="qrCode"
          width="200"
          height="200"
          alt="QR"
          class="rounded-lg bg-white p-2"
        />
        <div class="w-full">
          <span class="muted text-xs">{{ t.twoFactorManualKey }}</span>
          <code class="panel-card mt-1 block break-all px-3 py-2 text-sm">{{ secret }}</code>
        </div>
      </div>
      <div class="flex justify-center">
        <InputOtp v-model="code" :length="6" integer-only :aria-label="t.twoFactorCode" />
      </div>
      <div class="flex justify-end gap-2">
        <Button :label="t.cancel" severity="secondary" text @click="visible = false" />
        <Button
          type="submit"
          :label="t.enableTwoFactor"
          icon="pi pi-check"
          :loading="busy"
          :disabled="code.length !== 6"
        />
      </div>
    </form>

    <form v-else-if="step === 'confirm'" class="space-y-5" @submit.prevent="confirmSensitive">
      <p class="muted m-0 text-sm">{{ t.twoFactorConfirmHint }}</p>
      <Fluid class="space-y-5">
        <FloatLabel variant="on">
          <Password
            id="mfa-confirm-password"
            v-model="password"
            :feedback="false"
            toggle-mask
            autocomplete="current-password"
          />
          <label for="mfa-confirm-password">{{ t.currentPassword }}</label>
        </FloatLabel>
        <FloatLabel variant="on">
          <InputText
            id="mfa-confirm-code"
            v-model="code"
            autocomplete="one-time-code"
            class="font-mono"
          />
          <label for="mfa-confirm-code">{{ t.codeOrRecovery }}</label>
        </FloatLabel>
      </Fluid>
      <div class="flex justify-end gap-2">
        <Button :label="t.cancel" severity="secondary" text @click="visible = false" />
        <Button
          type="submit"
          :label="mode === 'disable' ? t.disableTwoFactor : t.regenerateRecoveryCodes"
          :severity="mode === 'disable' ? 'danger' : undefined"
          :loading="busy"
          :disabled="!password || code.trim().length < 6"
        />
      </div>
    </form>

    <div v-else class="space-y-4">
      <Message severity="warn" :closable="false">{{ t.recoveryCodesHint }}</Message>
      <ol class="recovery-grid">
        <li v-for="item in recoveryCodes" :key="item" class="font-mono">{{ item }}</li>
      </ol>
      <div class="flex flex-wrap justify-end gap-2">
        <Button
          :label="t.copyCodes"
          icon="pi pi-copy"
          severity="secondary"
          outlined
          @click="copyCodes"
        />
        <Button
          :label="t.downloadCodes"
          icon="pi pi-download"
          severity="secondary"
          outlined
          @click="downloadCodes"
        />
        <Button :label="t.codesSaved" icon="pi pi-check" @click="visible = false" />
      </div>
    </div>
  </Dialog>
</template>

<style scoped>
.recovery-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 16px;
  margin: 0;
  padding: 14px 16px 14px 36px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--app-surface-muted);
  font-size: 0.95rem;
}
</style>
