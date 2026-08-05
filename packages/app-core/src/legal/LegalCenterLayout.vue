<script setup lang="ts">
import { RouterLink, RouterView } from "vue-router";

import { legalDocuments } from "./catalog";
</script>

<template>
  <div class="legal-center">
    <header class="legal-center__header">
      <RouterLink class="legal-center__brand" to="/legal/privacy"
        >豆栈 DouStack</RouterLink
      >
      <span class="legal-center__title">法律中心</span>
      <nav class="legal-center__nav" aria-label="法律文件">
        <RouterLink
          v-for="document in legalDocuments"
          :key="document.id"
          :to="`/legal/${document.id}`"
        >
          {{ document.title }}
        </RouterLink>
      </nav>
    </header>
    <main class="legal-center__main">
      <RouterView />
    </main>
    <footer class="legal-center__footer">豆栈 DouStack 法律中心</footer>
  </div>
</template>

<style scoped>
.legal-center {
  min-height: 100dvh;
  background: #f6f7fb;
  color: #1d1d24;
}

.legal-center__header {
  display: flex;
  align-items: center;
  gap: 18px;
  min-height: 60px;
  padding: 0 32px;
  border-bottom: 1px solid #e3e6ee;
  background: #ffffff;
}

.legal-center__brand,
.legal-center__nav a {
  color: inherit;
  text-decoration: none;
}

.legal-center__brand {
  font-size: 15px;
  font-weight: 800;
}

.legal-center__title {
  padding-left: 18px;
  border-left: 1px solid #d9dde8;
  color: #66697a;
  font-size: 13px;
}

.legal-center__nav {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 16px;
}

.legal-center__nav a {
  color: #66697a;
  font-size: 13px;
}

.legal-center__nav a.router-link-active {
  color: #5b45b5;
  font-weight: 700;
}

.legal-center__main {
  width: min(900px, calc(100% - 32px));
  margin: 0 auto;
  padding: 48px 0 64px;
}

.legal-center__footer {
  padding: 20px 32px;
  border-top: 1px solid #e3e6ee;
  color: #777b8e;
  font-size: 12px;
  text-align: center;
}

@media (max-width: 720px) {
  .legal-center__header {
    align-items: flex-start;
    flex-wrap: wrap;
    gap: 12px;
    padding: 16px;
  }

  .legal-center__title {
    padding-left: 12px;
  }

  .legal-center__nav {
    flex-basis: 100%;
    justify-content: flex-start;
  }

  .legal-center__main {
    width: min(100% - 32px, 900px);
    padding: 32px 0 48px;
  }
}
</style>
