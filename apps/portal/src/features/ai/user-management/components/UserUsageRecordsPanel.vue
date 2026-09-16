<script setup lang="ts">
import RequestRecordsWorkspace from "../../usage/RequestRecordsWorkspace.vue";
import type { TenantEndUserItem } from "@/api/types/tenant";
import type { UserUsageFilters } from "../model";
defineProps<{ user: TenantEndUserItem | null; filters: UserUsageFilters; reloadKey: number }>();
</script>
<template><RequestRecordsWorkspace v-if="user" :key="`${user.userId}:${reloadKey}`" :user-id="user.userId" embedded /></template>
