export type PortalMarkdownInlineNode =
  | { type: "text"; text: string }
  | { type: "code"; text: string }
  | { type: "strong"; children: PortalMarkdownInlineNode[] }
  | { type: "em"; children: PortalMarkdownInlineNode[] }
  | { type: "del"; children: PortalMarkdownInlineNode[] }
  | { type: "link"; href: string; children: PortalMarkdownInlineNode[] }
  | { type: "br" };

export interface PortalMarkdownListItem {
  blocks: PortalMarkdownBlock[];
  checked: boolean | null;
}

export type PortalMarkdownBlock =
  | { type: "paragraph"; inlines: PortalMarkdownInlineNode[] }
  | { type: "heading"; depth: number; inlines: PortalMarkdownInlineNode[] }
  | { type: "code"; lang: string; text: string }
  | { type: "blockquote"; blocks: PortalMarkdownBlock[] }
  | { type: "list"; ordered: boolean; start: number; items: PortalMarkdownListItem[] }
  | { type: "hr" };

interface ListMarker {
  kind: "ordered" | "unordered";
  indent: number;
  start: number;
  content: string;
  checked: boolean | null;
}

const headingPattern = /^ {0,3}(#{1,6})[ \t]+(.+?)\s*#*\s*$/;
const horizontalRulePattern = /^ {0,3}(?:(?:-\s*){3,}|(?:_\s*){3,}|(?:\*\s*){3,})$/;
const fencedCodePattern = /^ {0,3}(`{3,}|~{3,})([^\n]*)$/;
const blockquotePattern = /^ {0,3}> ?(.*)$/;
const orderedListPattern = /^([ \t]{0,3})(\d+)[.)]\s+(.*)$/;
const unorderedListPattern = /^([ \t]{0,3})([-+*])\s+(.*)$/;
const taskListPattern = /^\[( |x|X)\]\s+(.*)$/;
const autoLinkPattern = /^(https?:\/\/[^\s<]+[^<.,:;"')\]\s])/;

export function parsePortalMarkdown(source: string): PortalMarkdownBlock[] {
  if (!source.trim()) {
    return [];
  }
  const normalized = source.replace(/\r\n?/g, "\n");
  return parseBlocks(normalized.split("\n"));
}

function parseBlocks(lines: string[]): PortalMarkdownBlock[] {
  const blocks: PortalMarkdownBlock[] = [];
  let index = 0;

  while (index < lines.length) {
    const line = lines[index];
    if (isBlank(line)) {
      index += 1;
      continue;
    }

    const fence = matchFence(line);
    if (fence) {
      const codeLines: string[] = [];
      index += 1;
      while (index < lines.length) {
        const candidate = lines[index];
        if (isFenceClose(candidate, fence.marker)) {
          index += 1;
          break;
        }
        codeLines.push(candidate);
        index += 1;
      }
      blocks.push({
        type: "code",
        lang: fence.lang,
        text: codeLines.join("\n")
      });
      continue;
    }

    const heading = line.match(headingPattern);
    if (heading) {
      blocks.push({
        type: "heading",
        depth: heading[1].length,
        inlines: parseInline(heading[2].trim())
      });
      index += 1;
      continue;
    }

    if (horizontalRulePattern.test(line)) {
      blocks.push({ type: "hr" });
      index += 1;
      continue;
    }

    if (blockquotePattern.test(line)) {
      const quoteLines: string[] = [];
      while (index < lines.length) {
        const quoteLine = lines[index];
        const match = quoteLine.match(blockquotePattern);
        if (!match) {
          break;
        }
        quoteLines.push(match[1]);
        index += 1;
      }
      blocks.push({
        type: "blockquote",
        blocks: parseBlocks(quoteLines)
      });
      continue;
    }

    const listMarker = readListMarker(line);
    if (listMarker) {
      const list = parseList(lines, index, listMarker);
      blocks.push(list.block);
      index = list.nextIndex;
      continue;
    }

    const paragraphLines: string[] = [];
    while (index < lines.length) {
      const current = lines[index];
      if (isBlank(current)) {
        break;
      }
      if (paragraphLines.length > 0 && startsStructuredBlock(current)) {
        break;
      }
      paragraphLines.push(current);
      index += 1;
    }
    blocks.push({
      type: "paragraph",
      inlines: parseInline(paragraphLines.join("\n"))
    });
  }

  return blocks;
}

function parseList(lines: string[], startIndex: number, firstMarker: ListMarker) {
  const items: PortalMarkdownListItem[] = [];
  let index = startIndex + 1;
  let currentItemLines = [firstMarker.content];
  let currentChecked = firstMarker.checked;

  while (index < lines.length) {
    const line = lines[index];
    const nextMarker = readListMarker(line);

    if (nextMarker && nextMarker.kind === firstMarker.kind && nextMarker.indent === firstMarker.indent) {
      items.push({
        checked: currentChecked,
        blocks: normalizeListItemBlocks(parseBlocks(currentItemLines))
      });
      currentItemLines = [nextMarker.content];
      currentChecked = nextMarker.checked;
      index += 1;
      continue;
    }

    if (isBlank(line)) {
      currentItemLines.push("");
      index += 1;
      continue;
    }

    if (countIndent(line) > firstMarker.indent) {
      currentItemLines.push(stripIndent(line, firstMarker.indent + 2));
      index += 1;
      continue;
    }

    break;
  }

  items.push({
    checked: currentChecked,
    blocks: normalizeListItemBlocks(parseBlocks(currentItemLines))
  });

  return {
    block: {
      type: "list" as const,
      ordered: firstMarker.kind === "ordered",
      start: firstMarker.start,
      items
    },
    nextIndex: index
  };
}

function normalizeListItemBlocks(blocks: PortalMarkdownBlock[]): PortalMarkdownBlock[] {
  return blocks.length > 0 ? blocks : [{ type: "paragraph", inlines: [] }];
}

function parseInline(text: string): PortalMarkdownInlineNode[] {
  const nodes: PortalMarkdownInlineNode[] = [];
  let buffer = "";
  let index = 0;

  const flushText = () => {
    if (!buffer) {
      return;
    }
    const last = nodes[nodes.length - 1];
    if (last?.type === "text") {
      last.text += buffer;
    } else {
      nodes.push({ type: "text", text: buffer });
    }
    buffer = "";
  };

  while (index < text.length) {
    const char = text[index];

    if (char === "\\") {
      if (index + 1 < text.length) {
        buffer += text[index + 1];
        index += 2;
      } else {
        buffer += char;
        index += 1;
      }
      continue;
    }

    if (char === "\n") {
      flushText();
      nodes.push({ type: "br" });
      index += 1;
      continue;
    }

    const codeFenceSize = countRun(text, index, "`");
    if (codeFenceSize > 0) {
      const delimiter = "`".repeat(codeFenceSize);
      const closeIndex = findClosingDelimiter(text, index + codeFenceSize, delimiter);
      if (closeIndex !== -1) {
        flushText();
        nodes.push({
          type: "code",
          text: text.slice(index + codeFenceSize, closeIndex)
        });
        index = closeIndex + codeFenceSize;
        continue;
      }
    }

    if (text.startsWith("**", index) || text.startsWith("__", index)) {
      const delimiter = text.slice(index, index + 2);
      const closeIndex = findClosingDelimiter(text, index + 2, delimiter);
      if (closeIndex !== -1) {
        flushText();
        nodes.push({
          type: "strong",
          children: parseInline(text.slice(index + 2, closeIndex))
        });
        index = closeIndex + 2;
        continue;
      }
    }

    if (text.startsWith("~~", index)) {
      const closeIndex = findClosingDelimiter(text, index + 2, "~~");
      if (closeIndex !== -1) {
        flushText();
        nodes.push({
          type: "del",
          children: parseInline(text.slice(index + 2, closeIndex))
        });
        index = closeIndex + 2;
        continue;
      }
    }

    if (char === "[" || char === "<" || char === "h") {
      const link = parseLink(text, index);
      if (link) {
        flushText();
        nodes.push(link.node);
        index = link.nextIndex;
        continue;
      }
    }

    if (char === "*" || char === "_") {
      const closeIndex = findClosingDelimiter(text, index + 1, char);
      if (closeIndex !== -1) {
        flushText();
        nodes.push({
          type: "em",
          children: parseInline(text.slice(index + 1, closeIndex))
        });
        index = closeIndex + 1;
        continue;
      }
    }

    buffer += char;
    index += 1;
  }

  flushText();
  return nodes;
}

function parseLink(text: string, startIndex: number) {
  if (text[startIndex] === "[") {
    const labelEnd = findMatchingCharacter(text, startIndex, "[", "]");
    if (labelEnd !== -1 && text[labelEnd + 1] === "(") {
      const hrefEnd = findMatchingCharacter(text, labelEnd + 1, "(", ")");
      if (hrefEnd !== -1) {
        const href = normalizeHref(extractLinkHref(text.slice(labelEnd + 2, hrefEnd)));
        if (href) {
          return {
            node: {
              type: "link" as const,
              href,
              children: parseInline(text.slice(startIndex + 1, labelEnd))
            },
            nextIndex: hrefEnd + 1
          };
        }
      }
    }
    return null;
  }

  if (text[startIndex] === "<") {
    const closeIndex = text.indexOf(">", startIndex + 1);
    if (closeIndex !== -1) {
      const href = normalizeHref(text.slice(startIndex + 1, closeIndex).trim());
      if (href) {
        return {
          node: {
            type: "link" as const,
            href,
            children: [{ type: "text" as const, text: href }]
          },
          nextIndex: closeIndex + 1
        };
      }
    }
    return null;
  }

  const match = text.slice(startIndex).match(autoLinkPattern);
  if (!match) {
    return null;
  }
  const href = normalizeHref(match[1]);
  if (!href) {
    return null;
  }
  return {
    node: {
      type: "link" as const,
      href,
      children: [{ type: "text" as const, text: href }]
    },
    nextIndex: startIndex + match[1].length
  };
}

function extractLinkHref(rawDestination: string): string {
  const trimmed = rawDestination.trim();
  const titleMatch = trimmed.match(/^(\S+)(?:\s+["'][^"']*["'])?$/);
  return titleMatch ? titleMatch[1] : trimmed;
}

function normalizeHref(rawHref: string): string | null {
  const trimmed = rawHref.trim().replace(/^<|>$/g, "");
  if (!trimmed) {
    return null;
  }
  if (trimmed.startsWith("/") || trimmed.startsWith("#")) {
    return trimmed;
  }
  try {
    const parsed = new URL(trimmed);
    if (parsed.protocol === "http:" || parsed.protocol === "https:" || parsed.protocol === "mailto:") {
      return trimmed;
    }
  } catch {
    return null;
  }
  return null;
}

function readListMarker(line: string): ListMarker | null {
  const ordered = line.match(orderedListPattern);
  if (ordered) {
    return buildListMarker("ordered", ordered[1], ordered[2], ordered[3]);
  }
  const unordered = line.match(unorderedListPattern);
  if (unordered) {
    return buildListMarker("unordered", unordered[1], "1", unordered[3]);
  }
  return null;
}

function buildListMarker(kind: "ordered" | "unordered", indentText: string, start: string, content: string): ListMarker {
  const task = content.match(taskListPattern);
  return {
    kind,
    indent: countIndent(indentText),
    start: Number.parseInt(start, 10) || 1,
    content: task ? task[2] : content,
    checked: task ? task[1].toLowerCase() === "x" : null
  };
}

function countIndent(line: string): number {
  let indent = 0;
  for (const char of line) {
    if (char === " ") {
      indent += 1;
      continue;
    }
    if (char === "\t") {
      indent += 4;
      continue;
    }
    break;
  }
  return indent;
}

function stripIndent(line: string, width: number): string {
  let removed = 0;
  let index = 0;
  while (index < line.length && removed < width) {
    if (line[index] === " ") {
      removed += 1;
      index += 1;
      continue;
    }
    if (line[index] === "\t") {
      removed += 4;
      index += 1;
      continue;
    }
    break;
  }
  return line.slice(index);
}

function startsStructuredBlock(line: string): boolean {
  return Boolean(matchFence(line) || line.match(headingPattern) || line.match(horizontalRulePattern) || line.match(blockquotePattern) || readListMarker(line));
}

function matchFence(line: string) {
  const match = line.match(fencedCodePattern);
  if (!match) {
    return null;
  }
  const info = match[2].trim();
  return {
    marker: match[1],
    lang: info.split(/\s+/)[0] || ""
  };
}

function isFenceClose(line: string, marker: string): boolean {
  const markerChar = marker[0];
  const markerLength = marker.length;
  const pattern = new RegExp(`^ {0,3}${escapeForRegExp(markerChar)}{${markerLength},}\\s*$`);
  return pattern.test(line);
}

function findClosingDelimiter(source: string, startIndex: number, delimiter: string): number {
  let searchIndex = startIndex;
  while (searchIndex < source.length) {
    const candidate = source.indexOf(delimiter, searchIndex);
    if (candidate === -1) {
      return -1;
    }
    if (!isEscaped(source, candidate)) {
      return candidate;
    }
    searchIndex = candidate + delimiter.length;
  }
  return -1;
}

function findMatchingCharacter(source: string, startIndex: number, open: string, close: string): number {
  let depth = 0;
  for (let index = startIndex; index < source.length; index += 1) {
    const char = source[index];
    if (isEscaped(source, index)) {
      continue;
    }
    if (char === open) {
      depth += 1;
      continue;
    }
    if (char === close) {
      depth -= 1;
      if (depth === 0) {
        return index;
      }
    }
  }
  return -1;
}

function isEscaped(source: string, index: number): boolean {
  let slashCount = 0;
  let cursor = index - 1;
  while (cursor >= 0 && source[cursor] === "\\") {
    slashCount += 1;
    cursor -= 1;
  }
  return slashCount % 2 === 1;
}

function countRun(source: string, startIndex: number, char: string): number {
  let count = 0;
  while (source[startIndex + count] === char) {
    count += 1;
  }
  return count;
}

function isBlank(line: string): boolean {
  return /^\s*$/.test(line);
}

function escapeForRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
