<script setup lang="ts">
import { DsTrendChart } from "@/shared/ui";
import type { DsTrendSeries } from "@/shared/ui/components/data/DsTrendChart.vue";
defineProps<{ labels: string[]; series: DsTrendSeries[]; valueFormatter?: (value: number) => string }>();
</script>
<template><DsTrendChart v-bind="$props" /></template>
