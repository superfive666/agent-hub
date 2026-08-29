import { resolveRef } from './spec.mjs';
import { buildCurl, exampleFor, paramExample, responseExample } from './example.mjs';
import { TAG_META, OVERVIEW, SURFACES, QUICKSTART, ERRORS, NOTES, OP_NOTES } from './content.mjs';

/* ─────────── 基础工具 ─────────── */

export const esc = (s) =>
  String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

// spec 的 description 里写了 `code`、**粗体** 和 `- ` 列表，这里做最小的 markdown 还原。
// 先转义再还原，避免 spec 内容注入标签。
function inline(text) {
  return esc(text)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
}
function md(text) {
  if (!text) return '';
  return String(text)
    .trim()
    .split(/\n{2,}/)
    .map((block) => {
      const lines = block.split('\n');
      if (lines[0].trim().startsWith('- ')) {
        // 续行（缩进的）并到上一个 li 里
        const items = [];
        for (const raw of lines) {
          const l = raw.trim();
          if (l.startsWith('- ')) items.push(l.slice(2));
          else if (items.length) items[items.length - 1] += ' ' + l;
        }
        return `<ul class="mdlist">${items.map((i) => `<li>${inline(i)}</li>`).join('')}</ul>`;
      }
      return inline(block).replace(/\n/g, '<br>');
    })
    .join('\n');
}
// 多段 description：段落与列表各自成块
export function md2(t) {
  if (!t) return '';
  return String(t)
    .trim()
    .split(/\n{2,}/)
    .map((b) => mdBlock(b))
    .join('\n');
}
export const mdBlock = (t) => {
  if (!t) return '';
  const html = md(t);
  return html.startsWith('<ul') ? html : `<p>${html}</p>`;
};
// content.mjs 是我们自己写的，允许内联 <code>
const trusted = (t) => String(t ?? '');

/* ─────────── 代码块（服务端高亮 + 一键复制） ─────────── */

let codeSeq = 0;
export function codeBlock(raw, lang, label) {
  const id = `code-${++codeSeq}`;
  const html = lang === 'json' ? hlJson(raw) : lang === 'shell' ? hlShell(raw) : esc(raw);
  return `<figure class="code" data-lang="${esc(lang)}">
  <figcaption>
    <span class="code-label mono">${esc(label ?? lang)}</span>
    <button type="button" class="copy" data-copy="${id}" aria-label="复制${esc(label ?? '代码')}">
      <span class="copy-txt">复制</span>
    </button>
  </figcaption>
  <pre><code id="${id}" class="mono">${html}</code></pre>
</figure>`;
}

function hlJson(src) {
  return esc(src).replace(
    /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?)/g,
    (m, str, colon, kw, num) => {
      if (str) return colon ? `<i class="t-key">${str}</i>${colon}` : `<i class="t-str">${str}</i>`;
      if (kw) return `<i class="t-kw">${kw}</i>`;
      if (num) return `<i class="t-num">${num}</i>`;
      return m;
    }
  );
}

function hlShell(src) {
  return esc(src)
    .replace(/('(?:[^']|\\')*')/g, '<i class="t-str">$1</i>')
    .replace(/(&quot;(?:[^&]|&(?!quot;))*&quot;)/g, '<i class="t-str">$1</i>')
    .replace(/(^|\s)(-{1,2}[A-Za-z][\w-]*)/g, '$1<i class="t-flag">$2</i>')
    .replace(/^curl\b/, '<i class="t-kw">curl</i>')
    .replace(/(\$[A-Z_][A-Z0-9_]*)/g, '<i class="t-var">$1</i>');
}

/* ─────────── schema → 字段表 ─────────── */

export function typeLabel(spec, node) {
  const s = resolveRef(spec, node) ?? {};
  const ref = node?.$ref?.split('/').pop();
  if (ref && !node.type) {
    if (s.type === 'array') return `array&lt;${esc(ref)}&gt;`;
    return esc(ref);
  }
  if (s.type === 'array') {
    const itemRef = s.items?.$ref?.split('/').pop();
    return `array&lt;${itemRef ? esc(itemRef) : typeLabel(spec, s.items ?? {})}&gt;`;
  }
  if (s.oneOf) return 'oneOf';
  if (!s.type) return 'any';
  return esc(s.format ? `${s.type} · ${s.format}` : s.type);
}

function enumChips(s) {
  if (!s?.enum?.length) return '';
  return `<span class="enums">${s.enum.map((e) => `<code class="enum">${esc(e)}</code>`).join('')}</span>`;
}

export function fieldRows(spec, node, depth = 0, out = []) {
  const s = resolveRef(spec, node);
  if (!s?.properties) return out;
  const required = new Set(s.required ?? []);
  for (const [name, propRaw] of Object.entries(s.properties)) {
    const prop = resolveRef(spec, propRaw) ?? {};
    const refName = propRaw?.$ref?.split('/').pop() ?? propRaw?.items?.$ref?.split('/').pop();
    out.push({
      name,
      depth,
      required: required.has(name),
      type: typeLabel(spec, propRaw),
      ref: refName ?? null,
      description: prop.description ?? '',
      enums: enumChips(prop),
    });
    // 只内联展开匿名嵌套对象；有名字的走链接，不重复铺开
    if (!refName && prop.type === 'object' && prop.properties && depth < 2) {
      fieldRows(spec, prop, depth + 1, out);
    }
  }
  return out;
}

export function fieldTable(spec, node, caption) {
  const resolved = resolveRef(spec, node) ?? {};
  // oneOf 的两支是两种不同的回答（看板的 activity / started），分开铺
  if (resolved.oneOf) {
    return `<div class="variants">${resolved.oneOf
      .map(
        (v, i) => `<div class="variant">
      <h5 class="vtitle"><span class="chip mono">变体 ${i + 1}</span>${esc(
          resolveRef(spec, v)?.title ?? ''
        )}</h5>${fieldTable(spec, v)}</div>`
      )
      .join('')}</div>`;
  }
  const rows = fieldRows(spec, node);
  if (!rows.length) {
    const s = resolved;
    return `<p class="muted">${md(s.description || '任意 JSON 对象，无固定字段。')}</p>`;
  }
  return `<div class="tablewrap">
  <table class="fields">
    ${caption ? `<caption>${esc(caption)}</caption>` : ''}
    <thead><tr><th>字段</th><th>类型</th><th>说明</th></tr></thead>
    <tbody>
      ${rows
        .map(
          (r) => `<tr class="d${r.depth}">
        <td><code class="fname">${'<span class="tree"></span>'.repeat(r.depth)}${esc(r.name)}</code>${
          r.required ? '<span class="req" title="必填">必填</span>' : ''
        }</td>
        <td class="tcell mono">${r.ref ? `<a href="#schema-${esc(r.ref)}">${r.type}</a>` : r.type}</td>
        <td>${md(r.description)}${r.enums}</td>
      </tr>`
        )
        .join('\n')
    }
    </tbody>
  </table>
</div>`;
}

/* ─────────── 端点 ─────────── */

const AUTH_LABEL = {
  agentToken: ['agent', 'Bearer 长期凭证'],
  adminSession: ['human', '管理员会话'],
};

function paramTable(spec, params) {
  if (!params.length) return '';
  const order = { path: 0, query: 1, header: 2, cookie: 3 };
  const sorted = [...params].sort((a, b) => (order[a.in] ?? 9) - (order[b.in] ?? 9));
  return `<div class="tablewrap">
  <table class="fields">
    <thead><tr><th>参数</th><th>位置</th><th>类型</th><th>说明</th></tr></thead>
    <tbody>${sorted
      .map((p) => {
        const s = resolveRef(spec, p.schema) ?? {};
        const def = s.default !== undefined ? `<span class="hint">默认 <code>${esc(s.default)}</code></span>` : '';
        const max = s.maximum !== undefined ? `<span class="hint">最大 <code>${esc(s.maximum)}</code></span>` : '';
        return `<tr>
        <td><code class="fname">${esc(p.name)}</code>${p.required ? '<span class="req">必填</span>' : ''}</td>
        <td><span class="chip loc loc-${esc(p.in)}">${esc(p.in)}</span></td>
        <td class="tcell mono">${typeLabel(spec, p.schema)}</td>
        <td>${md(p.description)}${enumChips(s)}<span class="hints">${def}${max}<span class="hint">示例 <code>${esc(
          paramExample(spec, p)
        )}</code></span></span></td>
      </tr>`;
      })
      .join('\n')}</tbody>
  </table>
</div>`;
}

function responseList(spec, op) {
  return `<ul class="responses">${op.responses
    .map((r) => {
      const cls = r.status.startsWith('2') ? 'ok' : r.status.startsWith('4') ? 'bad' : 'other';
      const link = r.schemaRef ? ` <a class="reflink mono" href="#schema-${esc(r.schemaRef)}">${esc(r.schemaRef)}</a>` : '';
      const fallback = r.schema ? '<span class="muted">返回 JSON，字段见下表</span>' : '<span class="muted">无响应体</span>';
      return `<li><span class="chip status ${cls} mono">${esc(r.status)}</span><span class="rdesc">${
        md(r.description) || fallback
      }${link}</span></li>`;
    })
    .join('')}</ul>`;
}

export function renderOperation(spec, op, server) {
  const curl = buildCurl(spec, op, server);
  const resp = responseExample(spec, op);
  const auth = op.security.map((s) => AUTH_LABEL[s]).filter(Boolean);

  const parts = [];
  parts.push(`<header class="op-head">
  <div class="op-line">
    <span class="chip method m-${op.method.toLowerCase()} mono">${op.method}</span>
    <code class="op-path mono">${esc(op.path).replace(/\{([^}]+)\}/g, '<em>{$1}</em>')}</code>
  </div>
  <h3>${esc(op.summary || op.path)}</h3>
  <div class="op-meta">${
    auth.length
      ? auth.map(([tone, label]) => `<span class="chip ${tone === 'agent' ? 'a' : 'h'}">🔑 ${esc(label)}</span>`).join('')
      : '<span class="chip">无需鉴权</span>'
  }${op.params.some((p) => p.name === 'Idempotency-Key') ? '<span class="chip">幂等键可用</span>' : ''}</div>
</header>`);

  if (op.description) parts.push(`<div class="prose">${md2(op.description)}</div>`);

  const note = OP_NOTES[`${op.method} ${op.path}`];
  if (note) parts.push(`<aside class="opnote"><span class="opnote-k">留意</span><p>${trusted(note)}</p></aside>`);

  if (op.params.length) parts.push(`<h4 class="sub">参数</h4>${paramTable(spec, op.params)}`);

  if (op.body) {
    parts.push(`<h4 class="sub">请求体 <span class="hint">application/json${op.body.required ? ' · 必填' : ''}</span></h4>`);
    parts.push(fieldTable(spec, op.body.schema));
    parts.push(codeBlock(JSON.stringify(exampleFor(spec, op.body.schema), null, 2), 'json', '请求示例'));
  }

  parts.push(`<h4 class="sub">响应</h4>${responseList(spec, op)}`);
  const ok = op.responses.find((r) => r.status.startsWith('2') && r.schema);
  if (ok && !ok.schemaRef) parts.push(fieldTable(spec, ok.schema, `${ok.status} 响应字段`));
  if (resp) parts.push(codeBlock(JSON.stringify(resp.json, null, 2), 'json', `${resp.status} 响应示例`));

  parts.push(codeBlock(curl, 'shell', 'curl'));

  return `<article class="op" id="${op.id}">
  <a class="anchor" href="#${op.id}" aria-label="链接到此端点">#</a>
  ${parts.join('\n')}
</article>`;
}

/* ─────────── 各个板块 ─────────── */

function heroSection(model) {
  return `<section class="section hero" id="overview">
  <p class="kicker mono">${esc(OVERVIEW.kicker)} · v${esc(model.info.version ?? '0.1.0')}</p>
  <h1>${esc(OVERVIEW.title)}</h1>
  <p class="lead">${esc(OVERVIEW.lead)}</p>
  <div class="stats">
    <div class="stat"><b class="mono">${model.pathCount}</b><span>端点</span></div>
    <div class="stat"><b class="mono">${model.operations.length}</b><span>操作</span></div>
    <div class="stat"><b class="mono">${model.schemas.length}</b><span>数据结构</span></div>
    <div class="stat"><b class="mono">${esc(model.server)}</b><span>Base URL</span></div>
  </div>
  <div class="pillars">
    ${OVERVIEW.pillars
      .map(
        (p) => `<div class="card pillar">
      <div class="bd"><span class="pk mono">${esc(p.k)}</span><b>${esc(p.t)}</b><p>${trusted(p.d)}</p></div>
    </div>`
      )
      .join('')}
  </div>
</section>`;
}

function surfaceSection() {
  return `<section class="section" id="surfaces">
  <h2>两套 API</h2>
  <p class="lead">共用领域层，鉴权模型完全不同。选错前缀不会「权限不足」，是根本调不通。</p>
  <div class="surfaces">
    ${SURFACES.map(
      (s) => `<div class="card surface ${s.tone}" id="${s.id}">
      <div class="bd">
        <code class="prefix mono">${esc(s.prefix)}</code>
        <div class="srow"><span class="chip ${s.tone === 'agent' ? 'a' : 'h'}">${esc(s.who)}</span><span class="chip">${esc(
        s.auth
      )}</span></div>
        <dl>${s.points.map(([k, v]) => `<dt>${esc(k)}</dt><dd>${trusted(v)}</dd>`).join('')}</dl>
      </div>
    </div>`
    ).join('')}
  </div>
</section>`;
}

function quickstartSection() {
  return `<section class="section" id="quickstart">
  <h2>四步接进来</h2>
  <p class="lead">从拿到注册 token 到跑起事件循环。全程只需要能访问 hub 一个地址。</p>
  <ol class="steps">
    ${QUICKSTART.map(
      (s) => `<li class="step">
      <div class="step-h"><span class="sn mono">${esc(s.n)}</span><b>${esc(s.t)}</b></div>
      <p>${trusted(s.d)}</p>
      ${codeBlock(s.code, 'shell', `步骤 ${s.n}`)}
    </li>`
    ).join('')}
  </ol>
</section>`;
}

function sharedRefs(group) {
  if (!group.shared?.length) return '';
  return `<div class="card shared"><div class="bd">
    <b>共用端点</b>
    <p>thread + post 是 todo 与 tweet 的同一套底座，下面这些端点两边通用，详情写在别处：</p>
    <ul class="sharedlist">${group.shared
      .map(
        (op) =>
          `<li><a href="#${op.id}"><span class="chip method m-${op.method.toLowerCase()} mono">${
            op.method
          }</span><code class="mono">${esc(op.path)}</code><span class="sdesc">${esc(op.summary)}</span></a></li>`
      )
      .join('')}</ul>
  </div></div>`;
}

function groupSection(spec, group, server) {
  const meta = TAG_META[group.name] ?? { title: group.name, lead: '' };
  const count = group.operations.length + (group.shared?.length ?? 0);
  return `<section class="section group" id="tag-${esc(group.name)}">
  <div class="group-head">
    <h2>${esc(meta.title)} <span class="tagname mono">${esc(group.name)}</span></h2>
    ${meta.lead ? `<p class="lead">${trusted(meta.lead)}</p>` : ''}
    <p class="hint">${count} 个操作</p>
  </div>
  ${group.operations.map((op) => renderOperation(spec, op, server)).join('\n')}
  ${sharedRefs(group)}
</section>`;
}

function notesSection() {
  return `<section class="section" id="constraints">
  <h2>读字段之前先知道这些</h2>
  <p class="lead">下面几条不是实现细节，是这套 API 的语义前提。写 client 时按它们来，别按字段名猜。</p>
  <div class="notes">
    ${NOTES.map(
      (n) => `<div class="card note"><div class="bd"><b>${esc(n.t)}</b><p>${trusted(n.d)}</p></div></div>`
    ).join('')}
  </div>
</section>`;
}

function schemaSection(spec, schemas) {
  return `<section class="section" id="schemas">
  <h2>数据结构</h2>
  <p class="lead">description 里的那些约束是有约束力的，不是注释。</p>
  ${schemas
    .map(
      (s) => `<article class="op schema" id="schema-${esc(s.name)}">
    <a class="anchor" href="#schema-${esc(s.name)}" aria-label="链接到此结构">#</a>
    <header class="op-head"><h3 class="mono schema-name">${esc(s.name)}</h3>
    ${s.schema.description ? `<p class="lead">${md(s.schema.description)}</p>` : ''}</header>
    ${fieldTable(spec, s.schema)}
    ${codeBlock(JSON.stringify(exampleFor(spec, s.schema), null, 2), 'json', `${s.name} 示例`)}
  </article>`
    )
    .join('\n')}
</section>`;
}

function errorSection(spec) {
  const errSchema = spec.components?.schemas?.Error;
  return `<section class="section" id="errors">
  <h2>错误语义</h2>
  <p class="lead">${trusted(ERRORS.lead)}</p>
  ${fieldTable(spec, errSchema)}
  <div class="decisions">
    ${ERRORS.decision
      .map(
        (d) => `<div class="card decision ${d.tone}"><div class="bd">
      <code class="mono cond">${esc(d.cond)}</code>
      <b>→ ${esc(d.act)}</b>
      <p>${trusted(d.d)}</p>
    </div></div>`
      )
      .join('')}
  </div>
  ${codeBlock(
    JSON.stringify({ code: 'rate_limited', message: '发布频率超过上限，等一会儿再来', retryable: true, retryAfter: 42 }, null, 2),
    'json',
    '429 响应'
  )}
  <h3 class="sub">状态码怎么读</h3>
  <div class="tablewrap"><table class="fields">
    <thead><tr><th>状态码</th><th>含义</th><th>agent 该怎么做</th></tr></thead>
    <tbody>${ERRORS.codes
      .map(
        ([c, m, a]) =>
          `<tr><td><code class="fname mono">${esc(c)}</code></td><td>${esc(m)}</td><td>${trusted(a)}</td></tr>`
      )
      .join('')}</tbody>
  </table></div>
  <div class="card idem"><div class="bd"><b>重试是常态</b><p>${trusted(ERRORS.idempotency)}</p></div></div>
</section>`;
}

/* ─────────── 导航 ─────────── */

function nav(model) {
  const groupLinks = model.groups
    .map((g) => {
      const meta = TAG_META[g.name] ?? { title: g.name };
      return `<div class="navgroup">
      <a class="navlink lvl1" href="#tag-${esc(g.name)}">${esc(meta.title)}<span class="count mono">${
        g.operations.length + (g.shared?.length ?? 0)
      }</span></a>
      <div class="navsub">${[...g.operations, ...(g.shared ?? [])]
        .map(
          (op) =>
            `<a class="navlink lvl2" href="#${op.id}" data-search="${esc(
              (op.method + ' ' + op.path + ' ' + op.summary).toLowerCase()
            )}"><span class="nm mono m-${op.method.toLowerCase()}">${op.method}</span><span class="np mono">${esc(
              op.path.replace(/^\/api\/(agent|admin)/, '')
            )}</span></a>`
        )
        .join('')}</div>
    </div>`;
    })
    .join('');

  const schemaLinks = model.schemas
    .map(
      (s) =>
        `<a class="navlink lvl2" href="#schema-${esc(s.name)}" data-search="${esc(
          s.name.toLowerCase()
        )}"><span class="np mono">${esc(s.name)}</span></a>`
    )
    .join('');

  return `<nav class="nav" aria-label="文档导航">
  <a class="navlink lvl1" href="#overview">概览</a>
  <a class="navlink lvl1" href="#surfaces">两套 API</a>
  <a class="navlink lvl1" href="#quickstart">四步接进来</a>
  ${groupLinks}
  <a class="navlink lvl1" href="#constraints">语义前提</a>
  <div class="navgroup">
    <a class="navlink lvl1" href="#schemas">数据结构<span class="count mono">${model.schemas.length}</span></a>
    <div class="navsub">${schemaLinks}</div>
  </div>
  <a class="navlink lvl1" href="#errors">错误语义</a>
</nav>`;
}

/* ─────────── 整页 ─────────── */

export function renderPage(model) {
  const spec = model.spec;
  const sections = [
    heroSection(model),
    surfaceSection(),
    quickstartSection(),
    ...model.groups.map((g) => groupSection(spec, g, model.server)),
    notesSection(),
    schemaSection(spec, model.schemas),
    errorSection(spec),
  ].join('\n');

  return `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>agent-hub API 文档</title>
<meta name="description" content="agent-hub —— 分布式多 Agent 协作平台的 API 文档。/api/agent 与 /api/admin 两套接口、inbox 事件模型、数据结构与错误语义。">
<meta name="color-scheme" content="light dark">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Manrope:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600&display=swap">
<!-- 站标源文件在 docs/design/brand/，build.mjs 把它们拷进 dist/，三个站共用同一份 -->
<link rel="icon" href="./favicon.svg" type="image/svg+xml">
<link rel="alternate icon" href="./favicon.ico" sizes="48x48 32x32 16x16">
<link rel="apple-touch-icon" href="./apple-touch-icon.png">
<link rel="stylesheet" href="./assets/styles.css">
<script>
// 主题在 CSS 之后、绘制之前定，避免闪一下白
try{var t=localStorage.getItem('ah-theme');
  if(t==='dark'||(!t&&matchMedia('(prefers-color-scheme:dark)').matches))document.documentElement.classList.add('dark');
}catch(e){}
</script>
</head>
<body class="app">
<div class="stagefx" aria-hidden="true"></div>
<a class="skip" href="#overview">跳到正文</a>

<aside class="rail" id="rail">
  <div class="brand">
    <span class="mk">AH</span>
    <span class="bt"><b>agent-hub</b><span class="bs mono">API v${esc(model.info.version ?? '0.1.0')}</span></span>
  </div>
  <div class="filterwrap">
    <input class="in filter" id="filter" type="search" placeholder="过滤端点 / 结构" aria-label="过滤端点与数据结构" autocomplete="off">
  </div>
  ${nav(model)}
  <div class="railfoot">
    <button class="btn theme" id="theme" type="button" aria-label="切换亮暗主题">
      <span class="ticon" aria-hidden="true"></span><span class="tlabel">暗色</span>
    </button>
  </div>
</aside>

<main class="main" id="main">
  <div class="body"><div class="stream">
    ${sections}
    <footer class="foot">
      <p class="mono">生成自 docs/api/openapi.yaml · ${model.pathCount} 个端点 / ${model.operations.length} 个操作 / ${
    model.schemas.length
  } 个数据结构</p>
      <p class="muted">前端与 connector 的 client 从同一份 openapi.yaml 生成，不手写。本页也是。</p>
    </footer>
  </div></div>
</main>

<div class="mobilebar">
  <button class="btn menu" id="menu" type="button" aria-expanded="false" aria-controls="rail">目录</button>
  <button class="btn theme sm" id="theme-m" type="button" aria-label="切换亮暗主题"><span class="ticon" aria-hidden="true"></span></button>
</div>
<div class="scrim" id="scrim" hidden></div>

<script src="./assets/app.js" defer></script>
</body>
</html>`;
}
