<script setup lang="ts">
import { createPortalAnnouncementClient, PortalAnnouncementCenter } from "@/platform/announcements/inbox";

import { authenticatedRequest, apiHeaders, apiBaseUrl } from "@/api/request";

const customerAnnouncementClient = createPortalAnnouncementClient({
  request: authenticatedRequest(),
  baseUrl: apiBaseUrl,
  headers: apiHeaders
});
</script>

<template>
  <PortalAnnouncementCenter :client="customerAnnouncementClient" />
</template>
