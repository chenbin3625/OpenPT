// 极简 CDP 客户端，只用 Node 内置的 WebSocket（Node 22+ 全局可用），
// 不引入 puppeteer 等依赖——这个项目的前端本身就是零依赖。
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const CHROME_CANDIDATES = [
    process.env.CHROME_PATH,
    process.env.CHROME_BIN,
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/Applications/Chromium.app/Contents/MacOS/Chromium',
    '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge',
    '/usr/bin/google-chrome',
    '/usr/bin/google-chrome-stable',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
    '/snap/bin/chromium',
].filter(Boolean);

export function findChrome() {
    for (const p of CHROME_CANDIDATES) {
        try {
            if (fs.existsSync(p)) return p;
        } catch {
            // 忽略不可访问的候选路径
        }
    }
    return null;
}

const sleep = ms => new Promise(r => setTimeout(r, ms));

async function waitForJSON(url, timeoutMs = 30000) {
    const deadline = Date.now() + timeoutMs;
    let lastErr;
    while (Date.now() < deadline) {
        try {
            const res = await fetch(url);
            if (res.ok) return await res.json();
        } catch (err) {
            lastErr = err;
        }
        await sleep(150);
    }
    throw new Error(`等待 ${url} 超时：${lastErr?.message || 'no response'}`);
}

/** 启动无头 Chrome 并返回一个已连接到页面 target 的 CDP 会话。 */
export async function launchChrome({ port = 9333, chromePath } = {}) {
    const bin = chromePath || findChrome();
    if (!bin) throw new Error('未找到 Chrome/Chromium，可用 CHROME_PATH 指定可执行文件路径');

    const userDataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'openpt-uitest-'));
    const proc = spawn(bin, [
        `--remote-debugging-port=${port}`,
        `--user-data-dir=${userDataDir}`,
        '--headless=new',
        '--no-sandbox',
        '--disable-gpu',
        '--disable-dev-shm-usage',
        '--no-first-run',
        '--no-default-browser-check',
        '--disable-extensions',
        '--disable-background-networking',
        '--disable-component-update',
        // 关键：无头窗口在 macOS 上会被系统判为遮挡，进而把页面标成
        // visibilityState=hidden，requestAnimationFrame 随之停摆（悬浮卡片定位、
        // 抽屉与 toast 入场动画都依赖 rAF）。这三个开关关掉后台节流，
        // 让页面在整轮测试里保持活跃。
        // 注意：不要改用 Emulation.setFocusEmulationEnabled —— 在 macOS 无头下
        // 配合键盘事件会把整个浏览器进程卡死（连 /json/version 都不再响应）。
        '--disable-backgrounding-occluded-windows',
        '--disable-renderer-backgrounding',
        '--disable-background-timer-throttling',
        '--force-color-profile=srgb',
        '--window-size=1600,1000',
        // 断言里假定"跟随系统"解析为亮色，锁死避免跑测机器的系统主题影响结果
        '--force-light-mode',
        'about:blank',
    ], { stdio: 'ignore' });

    const cleanupDir = () => {
        try {
            fs.rmSync(userDataDir, { recursive: true, force: true });
        } catch {
            // 目录清理失败不影响测试结论
        }
    };

    try {
        const version = await waitForJSON(`http://127.0.0.1:${port}/json/version`);
        const session = await connect(version.webSocketDebuggerUrl);
        session.on('closed', cleanupDir);
        return {
            session,
            product: version.Browser,
            async close() {
                await session.close();
                proc.kill('SIGKILL');
                cleanupDir();
            },
        };
    } catch (err) {
        proc.kill('SIGKILL');
        cleanupDir();
        throw err;
    }
}

/**
 * 连接浏览器级 WebSocket，并用 flatten 模式（sessionId）操作页面 target，
 * 这样一条连接就够，不必为每个 target 再开 socket。
 */
async function connect(wsUrl) {
    const ws = new WebSocket(wsUrl);
    await new Promise((resolve, reject) => {
        ws.addEventListener('open', resolve, { once: true });
        ws.addEventListener('error', () => reject(new Error(`无法连接 CDP：${wsUrl}`)), { once: true });
    });

    let nextId = 1;
    const pending = new Map();
    const listeners = new Map(); // event name -> Set<fn>
    const closedHandlers = new Set();

    ws.addEventListener('message', ev => {
        let msg;
        try {
            msg = JSON.parse(ev.data);
        } catch {
            return;
        }
        if (msg.id && pending.has(msg.id)) {
            const { resolve, reject } = pending.get(msg.id);
            pending.delete(msg.id);
            if (msg.error) reject(new Error(`${msg.error.message} (code ${msg.error.code})`));
            else resolve(msg.result);
            return;
        }
        if (msg.method) {
            for (const fn of listeners.get(msg.method) || []) fn(msg.params, msg.sessionId);
        }
    });

    ws.addEventListener('close', () => {
        for (const { reject } of pending.values()) reject(new Error('CDP 连接已关闭'));
        pending.clear();
        for (const fn of closedHandlers) fn();
    });

    let sessionId = null;

    function send(method, params = {}, opts = {}) {
        const id = nextId++;
        const payload = { id, method, params };
        // 浏览器域（Target.*、Browser.*）不带 sessionId
        if (!opts.browserLevel && sessionId) payload.sessionId = sessionId;
        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => {
                if (!pending.has(id)) return;
                pending.delete(id);
                reject(new Error(`CDP ${method} 超时（15s）`));
            }, 15000);
            const settle = fn => value => {
                clearTimeout(timer);
                fn(value);
            };
            pending.set(id, { resolve: settle(resolve), reject: settle(reject) });
            ws.send(JSON.stringify(payload));
        });
    }

    // 附到第一个 page target
    const { targetInfos } = await send('Target.getTargets', {}, { browserLevel: true });
    const page = targetInfos.find(t => t.type === 'page');
    if (!page) throw new Error('未找到 page target');
    const attached = await send(
        'Target.attachToTarget',
        { targetId: page.targetId, flatten: true },
        { browserLevel: true },
    );
    sessionId = attached.sessionId;

    return {
        send,
        targetId: page.targetId,
        on(event, fn) {
            if (event === 'closed') {
                closedHandlers.add(fn);
                return () => closedHandlers.delete(fn);
            }
            if (!listeners.has(event)) listeners.set(event, new Set());
            listeners.get(event).add(fn);
            return () => listeners.get(event)?.delete(fn);
        },
        off(event, fn) {
            listeners.get(event)?.delete(fn);
        },
        async close() {
            try {
                ws.close();
            } catch {
                // 已关闭
            }
        },
    };
}
