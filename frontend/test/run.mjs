#!/usr/bin/env node
/**
 * Web UI 浏览器回归测试（零依赖，只用 Node 内置模块 + 本机 Chrome/Chromium）。
 *
 *   node test/run.mjs              同时验证 dev（原生 ESM）与 preview（构建产物）
 *   node test/run.mjs --preview    只验证构建产物
 *   node test/run.mjs --dev        只验证源码直跑
 *
 * 环境变量 CHROME_PATH 可指定浏览器可执行文件。
 * 找不到浏览器时默认跳过（退出码 0）；CI 里请加 --require-browser 让它直接失败。
 */
import { spawn } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createMockBackend } from './mock-backend.mjs';
import { findChrome, launchChrome } from './cdp.mjs';
import { createDriver } from './driver.mjs';
import { CHECKS } from './checks.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const frontendDir = path.resolve(here, '..');

const argv = process.argv.slice(2);
const requireBrowser = argv.includes('--require-browser');
const onlyPreview = argv.includes('--preview');
const onlyDev = argv.includes('--dev');

// 端口全部由内核分配：既避免和本机已占用端口冲突（上一次跑挂了也不会连带影响下一次），
// 也让多个 job 能并行跑而不打架。
let mockPort = 0;
let cdpPort = 0;

const sleep = ms => new Promise(r => setTimeout(r, ms));

const color = process.stdout.isTTY && !process.env.NO_COLOR;
const c = {
    green: s => (color ? `\x1b[32m${s}\x1b[0m` : s),
    red: s => (color ? `\x1b[31m${s}\x1b[0m` : s),
    dim: s => (color ? `\x1b[2m${s}\x1b[0m` : s),
    bold: s => (color ? `\x1b[1m${s}\x1b[0m` : s),
    yellow: s => (color ? `\x1b[33m${s}\x1b[0m` : s),
};

/** 监听内核分配的随机端口，返回实际端口号。 */
function listen(server) {
    return new Promise((resolve, reject) => {
        server.once('error', reject);
        server.listen(0, '127.0.0.1', () => resolve(server.address().port));
    });
}

/** 先占一个空闲端口再释放，供必须显式指定端口的子进程（Chrome / serve.js）使用。 */
async function freePort() {
    const net = await import('node:net');
    const probe = net.createServer();
    const port = await listen(probe);
    await new Promise(r => probe.close(r));
    return port;
}

/** 启动 dev / preview 静态服务器子进程，等到端口真正可访问再返回。 */
async function startServer({ port, preview }) {
    const args = [path.join('scripts', 'serve.js')];
    if (preview) args.push('--preview');
    const proc = spawn(process.execPath, args, {
        cwd: frontendDir,
        env: { ...process.env, PORT: String(port), OPENPT_BACKEND: `http://127.0.0.1:${mockPort}` },
        stdio: ['ignore', 'ignore', 'pipe'],
    });
    let stderr = '';
    proc.stderr.on('data', d => {
        stderr += d.toString();
    });

    const deadline = Date.now() + 15000;
    for (;;) {
        if (proc.exitCode !== null) {
            throw new Error(`serve.js（port ${port}）启动失败：${stderr.trim() || '进程已退出'}`);
        }
        try {
            const res = await fetch(`http://127.0.0.1:${port}/`);
            if (res.ok) {
                await res.text();
                break;
            }
            if (res.status === 503) {
                await res.text();
                throw new Error(`preview 模式缺少构建产物，请先运行 npm run build`);
            }
        } catch (err) {
            if (String(err.message).includes('构建产物')) throw err;
        }
        if (Date.now() > deadline) throw new Error(`serve.js（port ${port}）启动超时`);
        await sleep(150);
    }
    return proc;
}

const same = (a, b) => (typeof a === 'object' && a !== null
    ? JSON.stringify(a) === JSON.stringify(b)
    : a === b);

const show = v => (typeof v === 'string' ? v : JSON.stringify(v));

/**
 * 执行一条断言。
 * 默认轮询到通过或超时，这样不依赖固定 sleep；
 * check.settle 存在时改为"等待固定时长后判定一次"，用于反向断言。
 */
async function runCheck(check, driver) {
    if (check.act) await check.act(driver);

    if (check.settle) {
        await sleep(check.settle);
        const actual = await driver.evalJS(check.expr);
        return { ok: same(actual, check.expect), actual };
    }

    const timeout = check.timeout || 5000;
    const deadline = Date.now() + timeout;
    let actual;
    for (;;) {
        actual = await driver.evalJS(check.expr);
        if (same(actual, check.expect)) return { ok: true, actual };
        if (Date.now() > deadline) return { ok: false, actual, timedOut: true };
        await sleep(100);
    }
}

/** 在一个 URL 上跑完整套断言，同时收集页面级错误。 */
async function runSuite(label, url, session) {
    const driver = createDriver(session);
    const pageErrors = [];

    const unsubs = [
        session.on('Runtime.exceptionThrown', p => {
            const d = p.exceptionDetails;
            pageErrors.push(`未捕获异常：${d.exception?.description || d.text}`);
        }),
        session.on('Runtime.consoleAPICalled', p => {
            if (p.type !== 'error') return;
            const text = p.args.map(a => a.value ?? a.description ?? a.type).join(' ');
            pageErrors.push(`console.error：${text}`);
        }),
        session.on('Log.entryAdded', p => {
            if (p.entry.level !== 'error') return;
            pageErrors.push(`日志错误：${p.entry.text} ${p.entry.url || ''}`.trim());
        }),
    ];

    await session.send('Runtime.enable');
    await session.send('Log.enable');
    await session.send('Page.enable');
    await session.send('Emulation.setDeviceMetricsOverride', {
        width: 1600, height: 1000, deviceScaleFactor: 1, mobile: false,
    });

    // 无头模式只在"有人要画面"时才产生帧，没有帧就没有 requestAnimationFrame 回调，
    // 而悬浮卡片定位、抽屉与 toast 的入场动画都依赖 rAF。开一路极小的 screencast
    // 当作帧的消费者，让 rAF 在整轮测试里持续推进。
    // （不要改用 Emulation.setFocusEmulationEnabled：在 macOS 无头下配合键盘事件
    // 会把整个浏览器进程卡死，连 /json/version 都不再响应。）
    const stopFrames = session.on('Page.screencastFrame', frame => {
        // 必须 ack，否则 Chrome 在第一帧之后就不再推送
        session.send('Page.screencastFrameAck', { sessionId: frame.sessionId }).catch(() => {});
    });
    await session.send('Page.startScreencast', {
        format: 'jpeg', quality: 1, maxWidth: 32, maxHeight: 32, everyNthFrame: 1,
    });
    // 剪贴板断言需要权限，否则 navigator.clipboard.writeText 会被拒绝
    // （代码里有 execCommand 兜底，这里授予权限是为了两条路径都能覆盖）
    try {
        await session.send('Browser.grantPermissions', {
            permissions: ['clipboardReadWrite', 'clipboardSanitizedWrite'],
        }, { browserLevel: true });
    } catch {
        // 部分 Chrome 版本权限名不同，失败则依赖 execCommand 兜底
    }

    // 每个 suite 从干净状态开始：清掉上一轮写入的主题偏好
    await session.send('Page.navigate', { url });
    await sleep(200);
    await driver.evalJS(`try { localStorage.clear(); } catch {} return true`);
    await session.send('Page.navigate', { url });
    await session.send('Runtime.evaluate', { expression: '1', returnByValue: true });

    console.log(`\n${c.bold(`▶ ${label}`)} ${c.dim(url)}`);

    const failures = [];
    let passed = 0;
    let aborted = false;
    for (const check of CHECKS) {
        let result;
        try {
            result = await runCheck(check, driver);
        } catch (err) {
            result = { ok: false, actual: `<执行出错> ${err.message}` };
            if (/超时/.test(err.message)) {
                // CDP 超时时先判断浏览器进程是否整体卡死。若连 HTTP 端点都不响应，
                // 后面每条断言都会再等满 15s，白跑十几分钟，所以直接中断本轮。
                let alive = false;
                try {
                    const r = await fetch(`http://127.0.0.1:${cdpPort}/json/version`, {
                        signal: AbortSignal.timeout(4000),
                    });
                    await r.text();
                    alive = r.ok;
                } catch {
                    alive = false;
                }
                console.log(`      ${c.yellow('诊断')} 浏览器 HTTP 端点`
                    + (alive ? '仍可用：疑似渲染进程或页面卡住' : '无响应：浏览器进程整体卡死'));
                if (!alive) {
                    failures.push({
                        name: check.name,
                        expect: check.expect,
                        actual: '浏览器进程卡死，已中断本轮剩余断言',
                    });
                    console.log(`  ${c.red('✗')} ${check.name}（浏览器卡死，中断剩余断言）`);
                    aborted = true;
                    break;
                }
            }
        }
        if (result.ok) {
            passed++;
            console.log(`  ${c.green('✓')} ${c.dim(check.name)}`);
        } else {
            failures.push({ name: check.name, expect: check.expect, actual: result.actual });
            console.log(`  ${c.red('✗')} ${check.name}`);
            console.log(`      ${c.dim('期望')} ${show(check.expect)}`);
            console.log(`      ${c.dim('实际')} ${show(result.actual)}`);
        }
    }

    for (const un of unsubs) un();
    stopFrames();
    await session.send('Page.stopScreencast').catch(() => {});
    return { label, passed, failures, pageErrors, aborted };
}

async function main() {
    if (!findChrome()) {
        const msg = '未找到 Chrome/Chromium：跳过 Web UI 浏览器回归测试（可用 CHROME_PATH 指定）';
        if (requireBrowser) {
            console.error(c.red(`✗ ${msg}`));
            console.error(c.red('  已指定 --require-browser，视为失败。'));
            process.exit(1);
        }
        console.log(c.yellow(`⚠ ${msg}`));
        return;
    }

    const suites = [];
    if (!onlyPreview) suites.push({ label: 'dev（源码原生 ESM）', preview: false });
    if (!onlyDev) suites.push({ label: 'preview（构建产物）', preview: true });

    const mock = createMockBackend();
    const servers = [];
    let chrome;

    try {
        mockPort = await listen(mock);
        for (const s of suites) {
            s.port = await freePort();
            servers.push(await startServer(s));
        }

        cdpPort = await freePort();
        chrome = await launchChrome({ port: cdpPort });
        console.log(c.dim(`浏览器：${chrome.product}`));
        console.log(c.dim(`断言数：${CHECKS.length} × ${suites.length} 套`));

        const results = [];
        for (const s of suites) {
            const r = await runSuite(s.label, `http://127.0.0.1:${s.port}/`, chrome.session);
            results.push(r);
            if (r.aborted) break; // 浏览器已卡死，后续 suite 跑不了
        }

        console.log(`\n${c.bold('汇总')}`);
        let failed = 0;
        for (const r of results) {
            const bad = r.failures.length + r.pageErrors.length;
            failed += bad;
            const line = `${r.passed}/${CHECKS.length} 通过`;
            console.log(`  ${bad === 0 ? c.green('✓') : c.red('✗')} ${r.label}：${line}`
                + (r.aborted ? c.red('（浏览器卡死，提前中断）') : '')
                + (r.pageErrors.length ? c.red(`，${r.pageErrors.length} 条页面错误`) : ''));
            for (const e of r.pageErrors) console.log(`      ${c.red(e)}`);
        }

        if (failed > 0) {
            console.error(`\n${c.red(`✗ Web UI 回归测试失败（${failed} 项）`)}`);
            process.exitCode = 1;
            return;
        }
        console.log(`\n${c.green('✓ Web UI 回归测试全部通过')}`);
    } finally {
        // 顺序很重要：先断浏览器，页面的 SSE 连接才会释放；
        // 再杀静态服务器子进程；mock 的 close 会等连接排空，所以不 await。
        if (chrome) {
            try {
                await chrome.close();
            } catch {
                // 浏览器可能已退出
            }
        }
        for (const p of servers) p.kill('SIGKILL');
        mock.closeAllConnections?.();
        mock.close();
    }
}

main().catch(err => {
    console.error(c.red(`✗ ${err.stack || err.message}`));
    process.exit(1);
});
