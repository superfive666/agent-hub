#!/usr/bin/env node
// 读 ../docs/api/openapi.yaml，产出 dist/ 静态站。
// 除了 YAML 解析，其余只用 Node 22 内置能力。
import { cp, mkdir, rm, writeFile, stat } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { loadSpec, buildModel } from './src/lib/spec.mjs';
import { renderPage } from './src/lib/render.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const SPEC = join(here, '..', 'docs', 'api', 'openapi.yaml');
const DIST = join(here, 'dist');
const checkOnly = process.argv.includes('--check');

const t0 = performance.now();
const loaded = loadSpec(SPEC);
const model = buildModel(loaded);

for (const w of model.warnings) console.warn(`  ! ${w}`);

// 生成的东西必须自洽：内部锚点全都要有落点
const anchors = new Set([
  ...model.operations.map((o) => o.id),
  ...model.schemas.map((s) => `schema-${s.name}`),
  ...model.groups.map((g) => `tag-${g.name}`),
  'overview', 'surfaces', 'quickstart', 'constraints', 'schemas', 'errors',
]);
const html = renderPage(model);
const broken = [...html.matchAll(/href="#([^"]+)"/g)].map((m) => m[1]).filter((a) => !anchors.has(a));
if (broken.length) {
  console.error('  ✗ 有指向不存在锚点的链接：', [...new Set(broken)].join(', '));
  process.exitCode = 1;
}
const dupIds = [];
const seen = new Set();
for (const m of html.matchAll(/\bid="([^"]+)"/g)) {
  if (seen.has(m[1])) dupIds.push(m[1]);
  seen.add(m[1]);
}
if (dupIds.length) {
  console.error('  ✗ 重复的 id：', [...new Set(dupIds)].join(', '));
  process.exitCode = 1;
}

if (checkOnly) {
  console.log(`  ✓ check：${model.pathCount} 端点 / ${model.operations.length} 操作 / ${model.schemas.length} 结构，锚点自洽`);
  process.exit(process.exitCode ?? 0);
}

await rm(DIST, { recursive: true, force: true });
await mkdir(join(DIST, 'assets'), { recursive: true });
await writeFile(join(DIST, 'index.html'), html, 'utf8');
await cp(join(here, 'src', 'styles.css'), join(DIST, 'assets', 'styles.css'));
await cp(join(here, 'src', 'app.js'), join(DIST, 'assets', 'app.js'));
// 规范里的原始 yaml 一起发出去，方便使用者生成 client
await cp(SPEC, join(DIST, 'openapi.yaml'));

const size = (await stat(join(DIST, 'index.html'))).size;
console.log(
  `  ✓ dist/index.html  ${(size / 1024).toFixed(1)} KB  ` +
    `${model.pathCount} 端点 · ${model.operations.length} 操作 · ${model.schemas.length} 结构 · ` +
    `${(performance.now() - t0).toFixed(0)}ms`
);
