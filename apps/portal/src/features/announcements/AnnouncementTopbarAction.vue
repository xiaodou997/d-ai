<script setup lang="ts">
import { createPortalAnnouncementClient, PortalAnnouncementCenter } from "@dai/app-core/announcements/inbox";

import { authenticatedRequest, portalHeaders, serviceBaseUrl } from "../../api/request";

const customerAnnouncementClient = createPortalAnnouncementClient({
  request: authenticatedRequest("urm"),
  baseUrl: serviceBaseUrl("urm"),
  headers: portalHeaders
});
</script>

<template>
  <PortalAnnouncementCenter :client="customerAnnouncementClient" />
</template>
