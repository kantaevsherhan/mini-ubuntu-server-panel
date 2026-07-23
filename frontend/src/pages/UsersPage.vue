<script setup lang="ts">
import { onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Tag from 'primevue/tag'
import Button from 'primevue/button'
import api from '../services/api'
import { formatDateTime } from '../services/dateTime'
import { useI18n } from '../services/i18n'
const users = ref<any[]>([]),
  loading = ref(true)
const { locale } = useI18n()
onMounted(async () => {
  users.value = (await api.get('/users')).data
  loading.value = false
})
</script>
<template>
  <section>
    <div class="mb-5 flex flex-wrap items-center gap-3">
      <h1 class="text-2xl font-semibold">Пользователи</h1>
      <span class="flex-1" /><Button label="Создать" icon="pi pi-user-plus" />
    </div>
    <DataTable
      :value="users"
      :loading="loading"
      size="small"
      scrollable
      scroll-height="calc(100vh - 180px)"
      striped-rows
      ><Column field="username" header="Username" sortable /><Column
        field="display_name"
        header="Display name" /><Column field="role" header="Роль"
        ><template #body="{ data }"><Tag :value="data.role" /></template></Column
      ><Column field="system_username" header="Ubuntu user" /><Column
        field="created_at"
        header="Создан"
        ><template #body="{ data }">{{ formatDateTime(data.created_at, locale) }}</template></Column
      ><Column header="Статус"
        ><template #body="{ data }"
          ><Tag
            :severity="data.is_active ? 'success' : 'secondary'"
            :value="data.is_active ? 'Active' : 'Disabled'" /></template></Column
      ><Column
        ><template #body><Button icon="pi pi-ellipsis-v" text rounded /></template></Column
    ></DataTable>
  </section>
</template>
