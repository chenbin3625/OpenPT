#!/usr/bin/env node
/**
 * 本地开发 / 预览服务器（零依赖）。
 *
 *   npm run dev      —— 直接从 src/ 提供原生 ES 模块，改完刷新即可，无需构建
 *   npm run preview  —— 提供 ../internal/web/dist 中的构建产物，验证上线形态
 *
 * 两种模式都把 /api/* 与 /openpt-icon.svg 反向代理到 Go 后端（metrics.webui 服务），
 * 否则同源请求会落到本服务器的 index.html 回退上，拿不到真实数据。
 */
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const preview = process.argv.includes('--preview');
const port = Number(process.env.PORT || 5173);
const backend = process.env.OPENPT_BACKEND || 'http://127.0.0.1:9090';
const distDir = path.resolve(root, '..', 'internal', 'web', 'dist');
const staticRoot = preview ? distDir : root;

const MIME = {
    '.html': 'text/html; charset=utf-8',
    '.js': 'text/javascript; charset=utf-8',
    '.css': 'text/css; charset=utf-8',
    '.json': 'application/json; charset=utf-8',
    '.svg': 'image/svg+xml',
    '.png': 'image/png',
    '.ico': 'image/x-icon',
    '.woff2': 'font/woff2',
};

const PROXY_PREFIXES = ['/api/', '/openpt-icon.svg', '/healthz', '/metrics'];
const shouldProxy = url => PROXY_PREFIXES.some(p => url === p || url.startsWith(p));

/** 开发模式下把构建占位符替换为直连源码的引用。 */
function devIndexHTML() {
    return fs.readFileSync(path.join(root, 'index.html'), 'utf8')
        .replace('<!--build:css-->', '<link rel="stylesheet" href="/src/styles/index.css">')
        .replace('<!--build:js-->', '<script type="module" src="/src/main.js"></script>');
}

function proxy(req, res) {
    const target = new URL(req.url, backend);
    const upstream = http.request(
        {
            hostname: target.hostname,
            port: target.port || 80,
            path: target.pathname + target.search,
            method: req.method,
            headers: { ...req.headers, host: target.host },
        },
        up => {
            res.writeHead(up.statusCode || 502, up.headers);
            up.pipe(res); // SSE 依赖流式转发，不能缓冲
        },
    );
    upstream.on('error', err => {
        res.writeHead(502, { 'content-type': 'text/plain; charset=utf-8' });
        res.end(`代理到 ${backend} 失败：${err.message}\n请确认后端已启动且开启了 metrics.webui。\n`);
    });
    req.pipe(upstream);
}

function sendFile(res, file) {
    const ext = path.extname(file).toLowerCase();
    res.writeHead(200, {
        'content-type': MIME[ext] || 'application/octet-stream',
        'cache-control': 'no-cache',
    });
    fs.createReadStream(file).pipe(res);
}

const server = http.createServer((req, res) => {
    const url = decodeURIComponent((req.url || '/').split('?')[0]);

    if (shouldProxy(url)) {
        proxy(req, res);
        return;
    }

    if (url === '/' || url === '/index.html') {
        if (preview) {
            const index = path.join(distDir, 'index.html');
            if (!fs.existsSync(index)) {
                res.writeHead(503, { 'content-type': 'text/plain; charset=utf-8' });
                res.end('尚无构建产物，请先运行 npm run build\n');
                return;
            }
            sendFile(res, index);
        } else {
            const html = devIndexHTML();
            res.writeHead(200, { 'content-type': MIME['.html'], 'cache-control': 'no-cache' });
            res.end(html);
        }
        return;
    }

    // 路径穿越防护：解析后必须仍在允许的根目录内
    const file = path.resolve(staticRoot, '.' + url);
    if (!file.startsWith(staticRoot + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) {
        res.writeHead(404, { 'content-type': 'text/plain; charset=utf-8' });
        res.end('404 Not Found\n');
        return;
    }
    sendFile(res, file);
});

server.listen(port, () => {
    console.log(`OpenPT Web UI ${preview ? '预览' : '开发'}服务器`);
    console.log(`  本地地址:  http://127.0.0.1:${port}/`);
    console.log(`  静态根目录: ${path.relative(process.cwd(), staticRoot) || '.'}`);
    console.log(`  API 代理:   ${PROXY_PREFIXES.join(', ')} → ${backend}`);
});
