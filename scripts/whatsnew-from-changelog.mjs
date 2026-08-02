#!/usr/bin/env node
// Generates data/whatsnew.json for the documentation site from CHANGELOG.md.
//
// Takes feature bullets (### Added, then ### Changed) from the most recent
// RELEASED version section — walking further back until it has MAX_ITEMS —
// so the homepage "What's New" panel always reflects the changelog instead
// of hand-maintained HTML. Run before `ssg` (see Makefile / deploy-docs.yml).

import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const MAX_ITEMS = 6;
const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const changelog = readFileSync(join(root, 'CHANGELOG.md'), 'utf8');

// Split into version sections: "## [2.11.4] - 2026-08-01" (skip Unreleased)
const sections = [...changelog.matchAll(/^## \[(\d+\.\d+\.\d+)\] - (\d{4}-\d{2}-\d{2})\n([\s\S]*?)(?=^## \[|\n*$(?![\s\S]))/gm)];
if (sections.length === 0) {
  console.error('whatsnew: no released versions found in CHANGELOG.md');
  process.exit(1);
}

// A feature bullet: "- **Title** — description..." (single- or multi-line)
function bullets(body, heading) {
  const m = body.match(new RegExp(`### ${heading}\\n([\\s\\S]*?)(?=\\n### |\\n## |$)`));
  if (!m) return [];
  const items = [];
  for (const b of m[1].split(/\n- /).map(s => s.trim()).filter(Boolean)) {
    const t = b.replace(/^- /, '');
    const title = t.match(/^\*\*(.+?)\*\*/);
    if (!title) continue;
    let desc = t.slice(title[0].length).replace(/^\s*[—–:-]\s*/, '');
    // First sentence only, markdown links/code stripped down to text
    desc = desc.split(/(?<=\.)\s/)[0]
      .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
      .replace(/`([^`]*)`/g, '<code>$1</code>')
      .replace(/\n\s*/g, ' ')
      .trim();
    items.push({ title: title[1].replace(/`([^`]*)`/g, '<code>$1</code>'), desc });
  }
  return items;
}

const latest = sections[0][1];
const date = sections[0][2];
const items = [];
for (const [, version, , body] of sections) {
  for (const it of [...bullets(body, 'Added'), ...bullets(body, 'Changed')]) {
    if (items.length >= MAX_ITEMS) break;
    items.push({ ...it, version });
  }
  if (items.length >= MAX_ITEMS) break;
}

mkdirSync(join(root, 'data'), { recursive: true });
writeFileSync(join(root, 'data', 'whatsnew.json'),
  JSON.stringify({ version: latest, date, items }, null, 2) + '\n');
console.log(`whatsnew: v${latest} (${date}), ${items.length} items`);
