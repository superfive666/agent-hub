// 读 openapi.yaml，归一化成渲染需要的模型。
//
// 一个坑：docs/api/openapi.yaml 里 `/api/admin/todos` 这个 key 出现了两次
// （一次带 get，一次带 post）。默认的 YAML 语义是「后者覆盖前者」，直接
// doc.toJS() 会把 `GET /api/admin/todos` 整个吃掉。这里从 AST 层把同名
// path 的 operation 合并回一起，并把重复 key 记进 warnings 报出来。
import { readFileSync } from 'node:fs';
import YAML from 'yaml';

const METHODS = ['get', 'post', 'put', 'patch', 'delete', 'head', 'options'];

export function loadSpec(file) {
  const src = readFileSync(file, 'utf8');
  const doc = YAML.parseDocument(src, { uniqueKeys: false });
  const warnings = [];

  const spec = doc.toJS();
  const pathsNode = doc.get('paths', true);
  const paths = new Map();

  if (pathsNode && pathsNode.items) {
    for (const item of pathsNode.items) {
      const key = String(item.key.value ?? item.key);
      const value = item.value?.toJSON?.() ?? {};
      if (paths.has(key)) {
        warnings.push(
          `openapi.yaml 中 path "${key}" 重复定义；已把两处的 operation 合并（YAML 默认语义会丢掉先出现的那份）`
        );
        Object.assign(paths.get(key), value);
      } else {
        paths.set(key, { ...value });
      }
    }
  }

  return { spec, paths, warnings, raw: src };
}

function refName(ref) {
  return ref?.startsWith('#/components/schemas/') ? ref.slice('#/components/schemas/'.length) : null;
}

export function resolveRef(spec, node) {
  let n = node;
  let guard = 0;
  while (n && n.$ref && guard++ < 10) {
    const parts = n.$ref.replace(/^#\//, '').split('/');
    let cur = spec;
    for (const p of parts) cur = cur?.[p];
    n = cur;
  }
  return n;
}

/** 把 spec 摊平成 {tags:[{name,operations:[...]}], schemas:[...]} */
export function buildModel(loaded) {
  const { spec, paths, warnings } = loaded;
  const operations = [];

  for (const [path, item] of paths) {
    const shared = item.parameters ?? [];
    for (const method of METHODS) {
      const op = item[method];
      if (!op) continue;
      const params = [...shared, ...(op.parameters ?? [])]
        .map((p) => resolveRef(spec, p))
        .filter(Boolean);

      const security = (op.security ?? spec.security ?? [])
        .flatMap((s) => Object.keys(s))
        .filter(Boolean);

      const body = op.requestBody?.content?.['application/json'];
      const responses = Object.entries(op.responses ?? {}).map(([status, r]) => {
        const content = r?.content?.['application/json'];
        return {
          status,
          description: r?.description ?? '',
          schema: content?.schema ?? null,
          schemaRef: refName(content?.schema?.$ref),
        };
      });

      operations.push({
        id: slug(method, path),
        method: method.toUpperCase(),
        path,
        tags: op.tags?.length ? op.tags : ['其他'],
        summary: op.summary ?? '',
        description: op.description ?? '',
        deprecated: !!op.deprecated,
        security,
        params,
        body: body ? { schema: body.schema, required: op.requestBody.required !== false } : null,
        responses,
      });
    }
  }

  // tag 顺序按 spec 里 tags: 的声明顺序，未声明的排在后面
  const declared = (spec.tags ?? []).map((t) => t.name);
  const seen = new Set();
  const order = [];
  for (const name of declared) if (operations.some((o) => o.tags.includes(name))) order.push(name);
  for (const op of operations)
    for (const t of op.tags) if (!order.includes(t) && !seen.has(t)) (seen.add(t), order.push(t));

  // 一个 operation 可能挂多个 tag（threads 相关的既是 todo 也是 tweet）。
  // 详情只在「主 tag」下渲染一次，其余 tag 里放一条交叉引用，避免 DOM 里出现重复 id。
  const groups = order.map((name) => ({
    name,
    operations: operations.filter((o) => o.tags[0] === name),
    shared: operations.filter((o) => o.tags[0] !== name && o.tags.includes(name)),
  }));

  const schemas = Object.entries(spec.components?.schemas ?? {}).map(([name, schema]) => ({
    name,
    schema,
  }));

  return {
    info: spec.info ?? {},
    server: spec.servers?.[0]?.url ?? 'https://hub.local',
    securitySchemes: spec.components?.securitySchemes ?? {},
    groups,
    operations,
    schemas,
    pathCount: paths.size,
    warnings,
    spec,
  };
}

export function slug(method, path) {
  return (
    method.toLowerCase() +
    '-' +
    path
      .replace(/^\/api\//, '')
      .replace(/[{}]/g, '')
      .replace(/[^a-zA-Z0-9]+/g, '-')
      .replace(/^-|-$/g, '')
  );
}
