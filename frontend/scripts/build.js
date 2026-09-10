#!/usr/bin/env node
/**
 * OpenPT Web UI 构建脚本（零依赖）。
 *
 * 职责：
 *   1. 从 src/main.js 出发解析 ES 模块依赖图，按依赖优先顺序拼成单个 IIFE；
 *   2. 从 src/styles/index.css 出发展开 @import，拼成单个样式表；
 *   3. 按内容哈希命名产物，写入 ../internal/web/dist/assets/，由 go:embed 内嵌；
 *   4. 用产物路径填充 index.html 中的 <!--build:css--> / <!--build:js--> 占位符。
 *
 * 之所以自己拼包而不是多文件原生 ESM：Go 端只暴露 /assets/ 与 /（见 internal/web/web.go），
 * 单文件产物请求数最少，也避免了给每个模块单独做哈希与 import 重写。
 */
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const srcDir = path.join(root, 'src');
const outDir = path.resolve(root, '..', 'internal', 'web', 'dist');
const assetsDir = path.join(outDir, 'assets');

const IMPORT_RE = /^import\s+[\s\S]*?from\s+['"]([^'"]+)['"];?[ \t]*\r?\n?/gm;
const NAMED_IMPORT_RE = /^import\s+\{([^}]*)\}\s+from\s+['"]([^'"]+)['"]/gm;
const EXPORT_NAME_RE = /^export\s+(?:async\s+)?(?:function|class|const|let|var)\s+([A-Za-z_$][\w$]*)/gm;
const EXPORT_RE = /^export\s+(?=(?:async\s+)?(?:function|class|const|let|var)\b)/gm;
const DECL_RE = /^(?:async\s+)?(?:function|class|const|let|var)\s+([A-Za-z_$][\w$]*)/gm;
const CSS_IMPORT_RE = /^@import\s+['"]([^'"]+)['"];[ \t]*\r?\n?/gm;

/** 解析 JS 模块图，返回依赖优先（post-order DFS）的模块列表。 */
function collectModules(entry) {
    const seen = new Map();
    const order = [];

    const visit = (file, importer) => {
        const abs = path.resolve(file);
        if (seen.has(abs)) return;
        seen.set(abs, true);
        if (!fs.existsSync(abs)) {
            throw new Error(`模块不存在: ${path.relative(root, abs)}（由 ${importer || 'entry'} 引入）`);
        }
        const code = fs.readFileSync(abs, 'utf8');
        const rel = path.relative(root, abs);

        // 扁平拼接只保留声明本身，因此 default 导出与重命名导出无法表达。
        if (/^export\s+(default|\{)/m.test(code)) {
            throw new Error(`${rel} 使用了 export default / export {...}，请改为在声明前加 export`);
        }

        const deps = [];
        const namedImports = [];
        for (const m of code.matchAll(IMPORT_RE)) {
            const spec = m[1];
            if (!spec.startsWith('.')) {
                // 前端刻意保持零依赖：出现裸模块说明引入了 npm 包，直接失败。
                throw new Error(`不允许的外部依赖 "${spec}"（${rel}）`);
            }
            deps.push(path.resolve(path.dirname(abs), spec));
        }
        for (const m of code.matchAll(NAMED_IMPORT_RE)) {
            const target = path.resolve(path.dirname(abs), m[2]);
            for (const raw of m[1].split(',')) {
                const name = raw.trim();
                if (!name) continue;
                if (name.includes(' as ')) {
                    throw new Error(`${rel} 使用了重命名导入 "${name}"，扁平拼接无法改名`);
                }
                namedImports.push({ name, target });
            }
        }

        for (const dep of deps) visit(dep, rel);
        order.push({
            file: abs,
            code,
            rel,
            namedImports,
            exports: new Set([...code.matchAll(EXPORT_NAME_RE)].map(m => m[1])),
        });
    };

    visit(entry, null);
    return order;
}

/**
 * 校验每个具名导入都能在目标模块里找到对应导出。
 * 扁平拼接后所有模块共享作用域，拼错的名字不会报错、只会在调用时才变成
 * undefined，因此必须在构建期挡住。
 */
function verifyImports(modules) {
    const byFile = new Map(modules.map(m => [m.file, m]));
    for (const mod of modules) {
        for (const { name, target } of mod.namedImports) {
            const dep = byFile.get(target);
            if (!dep) throw new Error(`${mod.rel} 引入了未解析的模块 ${target}`);
            if (!dep.exports.has(name)) {
                throw new Error(`${mod.rel} 引入的 "${name}" 未被 ${dep.rel} 导出`);
            }
        }
    }
}

/** 把模块列表拼成单个 IIFE，并检查跨模块的顶层重名。 */
function bundleJS(modules) {
    const owners = new Map();
    const chunks = [];

    for (const mod of modules) {
        const rel = mod.rel;
        const body = mod.code.replace(IMPORT_RE, '').replace(EXPORT_RE, '').trim();
        // 所有模块共享一个函数作用域，顶层重名会静默覆盖，因此在构建期拦住。
        for (const m of body.matchAll(DECL_RE)) {
            const name = m[1];
            if (owners.has(name)) {
                throw new Error(`顶层标识符 "${name}" 在 ${owners.get(name)} 与 ${rel} 中重复声明`);
            }
            owners.set(name, rel);
        }
        chunks.push(`// ==================== ${rel} ====================\n${body}\n`);
    }

    return `(function () {\n'use strict';\n\n${chunks.join('\n')}\n})();\n`;
}

/** 递归展开 CSS 的 @import，返回单个样式表。 */
function bundleCSS(entry) {
    const seen = new Set();

    const load = file => {
        const abs = path.resolve(file);
        if (seen.has(abs)) return '';
        seen.add(abs);
        if (!fs.existsSync(abs)) {
            throw new Error(`样式文件不存在: ${path.relative(root, abs)}`);
        }
        const rel = path.relative(srcDir, abs);
        const code = fs.readFileSync(abs, 'utf8');
        let imported = '';
        const body = code.replace(CSS_IMPORT_RE, (_, spec) => {
            imported += load(path.resolve(path.dirname(abs), spec));
            return '';
        });
        return `${imported}/* ==================== ${rel} ==================== */\n${body.trim()}\n\n`;
    };

    return load(entry).trim() + '\n';
}

const hash = content => createHash('sha256').update(content).digest('hex').slice(0, 10);
const kb = content => (Buffer.byteLength(content) / 1024).toFixed(1) + ' KB';

function main() {
    const modules = collectModules(path.join(srcDir, 'main.js'));
    verifyImports(modules);
    const js = bundleJS(modules);
    const css = bundleCSS(path.join(srcDir, 'styles', 'index.css'));

    const jsName = `app-${hash(js)}.js`;
    const cssName = `app-${hash(css)}.css`;

    // dist 是入库目录：先清空，避免旧哈希产物被 go:embed 一起打进二进制。
    fs.rmSync(outDir, { recursive: true, force: true });
    fs.mkdirSync(assetsDir, { recursive: true });
    fs.writeFileSync(path.join(assetsDir, jsName), js);
    fs.writeFileSync(path.join(assetsDir, cssName), css);

    // 语法自检：产物是普通脚本，node --check 能在构建期发现拼接后的语法问题。
    execFileSync(process.execPath, ['--check', path.join(assetsDir, jsName)], { stdio: 'inherit' });

    const html = fs.readFileSync(path.join(root, 'index.html'), 'utf8')
        .replace('<!--build:css-->', `<link rel="stylesheet" href="/assets/${cssName}">`)
        .replace('<!--build:js-->', `<script src="/assets/${jsName}" defer></script>`);
    if (html.includes('<!--build:')) {
        throw new Error('index.html 中仍有未替换的构建占位符');
    }
    fs.writeFileSync(path.join(outDir, 'index.html'), html);

    console.log(`✓ assets/${jsName}  ${kb(js)}`);
    console.log(`✓ assets/${cssName}  ${kb(css)}`);
    console.log(`✓ index.html  → ${path.relative(process.cwd(), outDir)}`);
}

try {
    main();
} catch (err) {
    console.error(`✗ 构建失败: ${err.message}`);
    process.exit(1);
}
