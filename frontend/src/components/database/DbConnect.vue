<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Select from 'primevue/select'
import Skeleton from 'primevue/skeleton'
import Tag from 'primevue/tag'
import {
  databaseClient,
  defaultPorts,
  engineLabels,
  forgetRecent,
  loadRecent,
  type Connection,
  type ContainerTarget,
  type Engine,
  type RecentConnection,
} from '../../services/databaseClient'
import { useI18n } from '../../services/i18n'

defineProps<{ loading: boolean }>()
const emit = defineEmits<{ connect: [Connection] }>()
const { t } = useI18n()

const form = reactive<Connection>({
  engine: 'postgres',
  host: '127.0.0.1',
  port: 5432,
  database: '',
  username: '',
  password: '',
  auth_source: 'admin',
  tls: false,
  container: '',
  schema: '',
})
const engines = (Object.keys(engineLabels) as Engine[]).map((value) => ({
  label: engineLabels[value],
  value,
}))
const containers = ref<ContainerTarget[]>([])
const containersLoading = ref(true)
const recent = ref<RecentConnection[]>(loadRecent())

const engineIcon: Record<Engine, string> = {
  postgres: 'pi pi-database',
  mysql: 'pi pi-server',
  mongodb: 'pi pi-sitemap',
}

onMounted(async () => {
  try {
    containers.value = await databaseClient.containers()
  } catch {
    containers.value = []
  } finally {
    containersLoading.value = false
  }
})

function connectContainer(target: ContainerTarget) {
  emit('connect', {
    engine: target.engine,
    host: target.host,
    port: target.port,
    database: target.database,
    username: '',
    password: '',
    auth_source: target.auth_source,
    tls: false,
    container: target.id,
    schema: '',
  })
}

function useRecent(item: RecentConnection) {
  Object.assign(form, item, { password: '' })
  if (item.container) emit('connect', { ...item, password: '' })
}

function remove(index: number) {
  forgetRecent(index)
  recent.value = loadRecent()
}

function submit() {
  emit('connect', { ...form, container: '' })
  form.password = ''
}
</script>

<template>
  <div class="grid grid-cols-[minmax(0,1fr)] gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.25fr)]">
    <div class="min-w-0 space-y-4">
      <section class="panel-card p-5">
        <div class="mb-1 flex items-center gap-2">
          <i class="pi pi-box text-[var(--p-primary-color)]" />
          <h2 class="section-title">{{ t.dbContainers }}</h2>
        </div>
        <p class="muted mb-4 text-sm">{{ t.dbContainersHint }}</p>
        <div v-if="containersLoading" class="space-y-2">
          <Skeleton height="3.5rem" /><Skeleton height="3.5rem" />
        </div>
        <p v-else-if="!containers.length" class="muted text-sm">{{ t.dbNoContainers }}</p>
        <div v-else class="space-y-2">
          <Button
            v-for="target in containers"
            :key="target.id"
            severity="secondary"
            outlined
            class="db-target"
            :disabled="loading"
            @click="connectContainer(target)"
          >
            <span class="db-target__icon"><i :class="engineIcon[target.engine]" /></span>
            <span class="min-w-0 flex-1 text-left">
              <span class="block truncate font-medium">{{ target.name }}</span>
              <span class="muted block truncate font-mono text-xs"
                >{{ target.username || '—' }}@{{ target.host }}:{{ target.port }}/{{
                  target.database
                }}</span
              >
            </span>
            <Tag
              :value="engineLabels[target.engine]"
              severity="secondary"
              class="hidden shrink-0 sm:inline-flex"
            />
            <Tag
              v-if="!target.has_password"
              :value="t.dbNoPassword"
              severity="warn"
              class="hidden shrink-0 sm:inline-flex"
            />
            <i class="pi pi-angle-right muted" />
          </Button>
        </div>
      </section>

      <section v-if="recent.length" class="panel-card p-5">
        <div class="mb-3 flex items-center gap-2">
          <i class="pi pi-history text-[var(--p-primary-color)]" />
          <h2 class="section-title">{{ t.dbRecent }}</h2>
        </div>
        <div class="space-y-1">
          <div v-for="(item, index) in recent" :key="index" class="flex items-center gap-2">
            <Button
              text
              severity="secondary"
              class="min-w-0 flex-1 !justify-start"
              :disabled="loading"
              @click="useRecent(item)"
            >
              <i :class="item.container ? 'pi pi-box' : engineIcon[item.engine]" />
              <span class="truncate font-mono text-xs"
                >{{ item.username || '—' }}@{{ item.host }}:{{ item.port }}/{{
                  item.database
                }}</span
              >
            </Button>
            <Button
              v-tooltip.left="t.dbForget"
              icon="pi pi-times"
              text
              rounded
              size="small"
              severity="secondary"
              :aria-label="t.dbForget"
              @click="remove(index)"
            />
          </div>
        </div>
      </section>
    </div>

    <section class="panel-card p-5">
      <div class="mb-4 flex items-center gap-2">
        <i class="pi pi-sign-in text-[var(--p-primary-color)]" />
        <h2 class="section-title">{{ t.dbManual }}</h2>
      </div>
      <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="submit">
        <div class="flex flex-col gap-2">
          <label for="db-engine">{{ t.databaseEngine }}</label>
          <Select
            v-model="form.engine"
            input-id="db-engine"
            :options="engines"
            option-label="label"
            option-value="value"
            @change="form.port = defaultPorts[form.engine]"
          />
        </div>
        <div class="grid grid-cols-[minmax(0,1fr)_7rem] gap-3">
          <div class="flex min-w-0 flex-col gap-2">
            <label for="db-host">{{ t.databaseHost }}</label>
            <InputText id="db-host" v-model="form.host" required :maxlength="253" />
          </div>
          <div class="flex flex-col gap-2">
            <label for="db-port">{{ t.dbPort }}</label>
            <InputNumber
              v-model="form.port"
              input-id="db-port"
              :min="1"
              :max="65535"
              :use-grouping="false"
              required
              fluid
            />
          </div>
        </div>
        <div class="flex flex-col gap-2">
          <label for="db-user">{{ t.username }}</label>
          <InputText id="db-user" v-model="form.username" autocomplete="off" :maxlength="256" />
        </div>
        <div class="flex flex-col gap-2">
          <label for="db-password">{{ t.password }}</label>
          <Password
            v-model="form.password"
            input-id="db-password"
            :feedback="false"
            toggle-mask
            autocomplete="off"
            fluid
          />
        </div>
        <div class="flex flex-col gap-2">
          <label for="db-name">{{ t.databaseName }}</label>
          <InputText
            id="db-name"
            v-model="form.database"
            required
            pattern="[A-Za-z0-9_][A-Za-z0-9_.$\-]{0,127}"
          />
        </div>
        <div v-if="form.engine === 'mongodb'" class="flex flex-col gap-2">
          <label for="db-auth">{{ t.databaseAuthSource }}</label>
          <InputText id="db-auth" v-model="form.auth_source" />
        </div>
        <div class="flex items-center gap-2 sm:col-span-2">
          <Checkbox v-model="form.tls" input-id="db-tls" binary />
          <label for="db-tls">{{ t.databaseTLS }}</label>
        </div>
        <div class="flex justify-end sm:col-span-2">
          <Button
            type="submit"
            :label="t.databaseConnect"
            icon="pi pi-sign-in"
            :loading="loading"
          />
        </div>
      </form>
    </section>
  </div>
</template>

<style scoped>
.db-target {
  width: 100%;
  justify-content: flex-start;
  gap: 12px;
  padding: 10px 12px;
  border-color: var(--app-border);
  color: var(--p-text-color);
  font-weight: 400;
}
.db-target:not(:disabled):hover {
  border-color: var(--p-primary-color);
}
.db-target__icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  border-radius: 8px;
  color: var(--p-primary-color);
  background: color-mix(in srgb, var(--p-primary-500) 14%, transparent);
}
</style>
