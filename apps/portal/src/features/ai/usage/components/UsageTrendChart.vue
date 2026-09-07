<script setup lang="ts">
import { computed } from "vue";
import { DsTrendChart } from "@/shared/ui";
import type { DailyTrendRowDTO } from "../model";
import { formatShortDate } from "../format";
interface TrendSeries { label: string; color: string; points: number[]; }
const props = defineProps<{ rows: DailyTrendRowDTO[]; series: TrendSeries[]; emptyText?: string }>();
const labels = computed(() => props.rows.map((row) => formatShortDate(row.date)));
const mapped = computed(() => props.series.map((series) => ({ label: series.label, color: series.color, values: series.points })));
</script>
<template><DsTrendChart :labels="labels" :series="mapped" :empty-text="props.emptyText || '暂无趋势数据'" /></template>
