import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const roots = ["README.md", "AGENTS.md", "docs"];
const files = roots.flatMap((root) => walk(path.join(rootDir, root))).filter((file) => file.endsWith(".md"));
const errors = [];
const docsDir = path.join(rootDir, "docs");

for (const file of files) {
  const source = fs.readFileSync(file, "utf8");
  for (const match of source.matchAll(/\[[^\]]+\]\(([^)]+)\)/g)) {
    const href = normalizeMarkdownHref(match[1]);
    const { linkPath, fragment } = splitHref(href);
    if (/^(https?:|mailto:)/.test(href)) {
      continue;
    }
    const target = linkPath ? path.normalize(path.join(path.dirname(file), linkPath)) : file;
    if (!fs.existsSync(target)) {
      errors.push(`${path.relative(rootDir, file)}: ${href} -> ${path.relative(rootDir, target)}`);
      continue;
    }
    if (fragment && target.endsWith(".md") && !markdownAnchors(target).has(normalizeAnchorFragment(fragment))) {
      errors.push(`${path.relative(rootDir, file)}: ${href} -> missing anchor in ${path.relative(rootDir, target)}`);
    }
  }
}

checkDocStructure();
checkDocIndexes();

if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exit(1);
}

console.log(`checked ${files.length} project markdown files; local links, anchors, and document indexes ok`);

function walk(target) {
  const stats = fs.statSync(target);
  if (!stats.isDirectory()) {
    return [target];
  }
  return fs.readdirSync(target).flatMap((entry) => walk(path.join(target, entry)));
}

function checkDocStructure() {
  const allowedRootDocs = new Set(["00-document-map.md", "README.md"]);
  const numberedDirPattern = /^(?:0[1-9]|[1-9][0-9])-[a-z0-9-]+$/;
  const topLevelEntries = fs.readdirSync(docsDir, { withFileTypes: true });

  for (const entry of topLevelEntries) {
    if (entry.isFile() && entry.name.endsWith(".md") && !allowedRootDocs.has(entry.name)) {
      errors.push(`docs/${entry.name}: design docs must live in a numbered docs subdirectory`);
    }
    if (entry.isDirectory() && directoryMarkdownFiles(path.join(docsDir, entry.name)).length > 0 && !numberedDirPattern.test(entry.name)) {
      errors.push(`docs/${entry.name}: docs directory names must start with a two-digit number`);
    }
  }

  for (const file of files.filter((file) => file.startsWith(docsDir + path.sep))) {
    const relative = path.relative(docsDir, file);
    const parts = relative.split(path.sep);
    if (parts.length === 1) {
      continue;
    }
    if (!/^[0-9]{2}-/.test(parts[0])) {
      errors.push(`${path.relative(rootDir, file)}: document must be under a numbered docs directory`);
    }
    if (!/^[0-9]+(?:-[a-z0-9]+)+\.md$/.test(parts[parts.length - 1])) {
      errors.push(`${path.relative(rootDir, file)}: document filename must start with a number and use kebab-case`);
    }
  }
}

function checkDocIndexes() {
  const indexes = [
    {
      name: "docs/00-document-map.md",
      links: localMarkdownLinks(path.join(docsDir, "00-document-map.md")),
    },
    {
      name: "docs/README.md",
      links: localMarkdownLinks(path.join(docsDir, "README.md")),
    },
  ];
  const designDocs = files
    .filter((file) => file.startsWith(docsDir + path.sep))
    .map((file) => path.relative(docsDir, file))
    .filter((relative) => relative !== "00-document-map.md" && relative !== "README.md")
    .sort();

  for (const relative of designDocs) {
    for (const index of indexes) {
      if (!index.links.has(relative)) {
        errors.push(`docs/${relative}: document must be linked from ${index.name}`);
      }
    }
  }
}

function localMarkdownLinks(file) {
  const source = fs.readFileSync(file, "utf8");
  const links = new Set();
  for (const match of source.matchAll(/\[[^\]]+\]\(([^)]+)\)/g)) {
    const { linkPath: link } = splitHref(normalizeMarkdownHref(match[1]));
    if (!link || /^(https?:|mailto:)/.test(link)) {
      continue;
    }
    const target = path.normalize(path.join(path.dirname(file), link));
    if (target.startsWith(docsDir + path.sep) && target.endsWith(".md")) {
      links.add(path.relative(docsDir, target));
    }
  }
  return links;
}

function directoryMarkdownFiles(dir) {
  return walk(dir).filter((file) => file.endsWith(".md"));
}

function normalizeMarkdownHref(rawHref) {
  const href = rawHref.trim();
  if (href.startsWith("<") && href.endsWith(">")) {
    return href.slice(1, -1);
  }
  return href;
}

function splitHref(href) {
  const hashIndex = href.indexOf("#");
  if (hashIndex === -1) {
    return { linkPath: href, fragment: "" };
  }
  return {
    linkPath: href.slice(0, hashIndex),
    fragment: href.slice(hashIndex + 1),
  };
}

function normalizeAnchorFragment(fragment) {
  try {
    return decodeURIComponent(fragment);
  } catch {
    return fragment;
  }
}

function markdownAnchors(file) {
  const source = fs.readFileSync(file, "utf8");
  const anchors = new Set();
  const counts = new Map();

  for (const line of source.split(/\r?\n/)) {
    const match = line.match(/^(#{1,6})\s+(.+?)\s*#*\s*$/);
    if (!match) {
      continue;
    }
    const text = stripInlineMarkdown(match[2]);
    const baseSlug = githubStyleSlug(text);
    const count = counts.get(baseSlug) ?? 0;
    counts.set(baseSlug, count + 1);
    anchors.add(count === 0 ? baseSlug : `${baseSlug}-${count}`);
  }

  for (const match of source.matchAll(/\bid=["']([^"']+)["']/g)) {
    anchors.add(match[1]);
  }

  return anchors;
}

function stripInlineMarkdown(text) {
  return text
    .replace(/`([^`]+)`/g, "$1")
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
    .replace(/[*_~]/g, "")
    .trim();
}

function githubStyleSlug(text) {
  return text
    .toLowerCase()
    .replace(/[^\p{Letter}\p{Number}\p{Mark}\s_-]/gu, "")
    .trim()
    .replace(/\s+/g, "-");
}
