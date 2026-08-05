import * as ElementPlusIconsVue from "@element-plus/icons-vue";
import zhCn from "element-plus/es/locale/lang/zh-cn";
import "element-plus/es/components/loading/style/css";
import "element-plus/es/components/message/style/css";
import "element-plus/es/components/message-box/style/css";
import "element-plus/es/components/notification/style/css";
import { createApp, type Component, type Plugin } from "vue";
import { createPinia } from "pinia";

import PortalAppRoot from "./PortalAppRoot.vue";

export interface BootstrapPortalAppOptions {
  root: Component;
  router: Plugin;
  icons?: Record<string, Component>;
  rootProps?: Record<string, unknown>;
  mountSelector?: string;
}

export interface BootstrapStandardPortalAppOptions {
  router: Plugin;
  rootProps?: Record<string, unknown>;
  showToaster?: boolean;
  mountSelector?: string;
}

export function bootstrapPortalApp(options: BootstrapPortalAppOptions) {
  const app = createApp(options.root, options.rootProps);
  app.use(createPinia());
  app.use(options.router);

  for (const [name, component] of Object.entries(options.icons ?? {}) as Array<[string, Component]>) {
    app.component(name, component);
  }

  app.mount(options.mountSelector || "#app");
  return app;
}

export function bootstrapStandardPortalApp(options: BootstrapStandardPortalAppOptions) {
  return bootstrapPortalApp({
    icons: ElementPlusIconsVue,
    mountSelector: options.mountSelector,
    root: PortalAppRoot,
    rootProps: {
      locale: zhCn,
      ...options.rootProps,
      ...(options.showToaster === undefined ? {} : { showToaster: options.showToaster })
    },
    router: options.router
  });
}
