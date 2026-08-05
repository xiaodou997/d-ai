import DOMPurify from "dompurify";
import { marked } from "marked";

marked.use({
  breaks: true,
  gfm: true
});

export function renderAnnouncementMarkdown(markdown: string): string {
  const html = marked.parse(markdown, { async: false });
  return DOMPurify.sanitize(html, { USE_PROFILES: { html: true } });
}
