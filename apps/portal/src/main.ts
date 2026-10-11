import router from "./router";
import { bootstrapStandardPortalApp } from "./platform/bootstrap";
// Programmatic feedback has no template for the component resolver to scan.
import "element-plus/es/components/message/style/css";
import "element-plus/es/components/message-box/style/css";
import "./styles/main.css";

bootstrapStandardPortalApp({ router });
