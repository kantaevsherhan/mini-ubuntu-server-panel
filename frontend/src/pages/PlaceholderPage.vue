<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
const route = useRoute()
const title = computed(() =>
  String(route.params.section || '').replace(/^./, (c) => c.toUpperCase()),
)
</script>
<template>
  <div>
    <h1 class="text-2xl font-semibold">{{ title }}</h1>
    <div class="panel-card p-8 text-center muted">
      <i class="pi pi-wrench text-3xl mb-3" />
      <p>Модуль подготовлен к подключению API.</p>
    </div>
  </div>
</template>
