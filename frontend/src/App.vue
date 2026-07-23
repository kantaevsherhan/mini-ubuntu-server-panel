<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import { useToast } from 'primevue/usetoast'
import ConfirmDialog from 'primevue/confirmdialog'
import Toast from 'primevue/toast'
import { onAPIError } from './services/apiErrors'
import { useI18n } from './services/i18n'

const toast = useToast()
const { t } = useI18n()
let unsubscribe: (() => void) | undefined

onMounted(() => {
  unsubscribe = onAPIError((error) => {
    toast.add({
      severity: 'error',
      summary: error.network ? t.value.networkError : t.value.requestError,
      detail: error.network ? t.value.networkErrorHint : `${t.value.errorCode}: ${error.code}`,
      life: 6000,
    })
  })
})
onBeforeUnmount(() => unsubscribe?.())
</script>

<template><Toast position="top-right" /><ConfirmDialog /><RouterView /></template>
