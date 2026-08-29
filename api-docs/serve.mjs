// 本地预览用的静态服务器。没有依赖，够看就行。
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';
import { dirname } from 'node:path';

const DIST = join(dirname(fileURLToPath(import.meta.url)), 'dist');
const PORT = Number(process.env.PORT ?? 4173);
const TYPES = {
  '.html': 'text/html; charset=utf-8', '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8', '.yaml': 'text/yaml; charset=utf-8',
  '.svg': 'image/svg+xml', '.json': 'application/json',
};

createServer(async (req, res) => {
  const url = new URL(req.url, 'http://localhost');
  let p = normalize(decodeURIComponent(url.pathname)).replace(/^(\.\.[/\\])+/, '');
  if (p.endsWith('/')) p += 'index.html';
  try {
    const buf = await readFile(join(DIST, p));
    res.writeHead(200, { 'content-type': TYPES[extname(p)] ?? 'application/octet-stream' });
    res.end(buf);
  } catch {
    res.writeHead(404, { 'content-type': 'text/plain; charset=utf-8' });
    res.end('404');
  }
}).listen(PORT, () => console.log(`  → http://localhost:${PORT}/`));
