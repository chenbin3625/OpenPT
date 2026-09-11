// 页面操作封装。点击/悬浮一律走 Input.dispatchMouseEvent，产生的是可信事件，
// 与 el.click() 不同——后者在剪贴板等需要用户激活的场景下会静默失败。
const sleep = ms => new Promise(r => setTimeout(r, ms));

export function createDriver(session) {
    /** 在页面里执行表达式体（需自行 return），返回反序列化后的值。 */
    async function evalJS(body) {
        const res = await session.send('Runtime.evaluate', {
            expression: `(() => { ${body} })()`,
            returnByValue: true,
            awaitPromise: true,
            userGesture: true,
        });
        if (res.exceptionDetails) {
            const d = res.exceptionDetails;
            throw new Error(`页面异常：${d.exception?.description || d.text}`);
        }
        return res.result.value;
    }

    /** 元素视口中心坐标；元素不存在或不可见时返回 null。 */
    async function centerOf(selector) {
        return evalJS(`
            const el = document.querySelector(${JSON.stringify(selector)});
            if (!el) return null;
            el.scrollIntoView({ block: 'center', inline: 'center' });
            const r = el.getBoundingClientRect();
            if (r.width === 0 || r.height === 0) return null;
            return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
        `);
    }

    async function centerOfNth(selector, index) {
        return evalJS(`
            const els = document.querySelectorAll(${JSON.stringify(selector)});
            const el = els[${index}];
            if (!el) return null;
            el.scrollIntoView({ block: 'center', inline: 'center' });
            const r = el.getBoundingClientRect();
            if (r.width === 0 || r.height === 0) return null;
            return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
        `);
    }

    async function waitForPoint(getPoint, label) {
        const deadline = Date.now() + 5000;
        for (;;) {
            const pt = await getPoint();
            if (pt) return pt;
            if (Date.now() > deadline) throw new Error(`等待可点击元素超时：${label}`);
            await sleep(100);
        }
    }

    async function mouseTo(point) {
        await session.send('Input.dispatchMouseEvent', {
            type: 'mouseMoved', x: point.x, y: point.y, buttons: 0,
        });
    }

    async function clickPoint(point) {
        await mouseTo(point);
        for (const type of ['mousePressed', 'mouseReleased']) {
            await session.send('Input.dispatchMouseEvent', {
                type, x: point.x, y: point.y, button: 'left', buttons: type === 'mousePressed' ? 1 : 0,
                clickCount: 1,
            });
        }
    }

    return {
        evalJS,
        sleep,

        async click(selector) {
            await clickPoint(await waitForPoint(() => centerOf(selector), selector));
        },

        async clickNth(selector, index) {
            await clickPoint(await waitForPoint(() => centerOfNth(selector, index), `${selector}[${index}]`));
        },

        async hover(selector) {
            await mouseTo(await waitForPoint(() => centerOf(selector), selector));
        },

        async hoverNth(selector, index) {
            await mouseTo(await waitForPoint(() => centerOfNth(selector, index), `${selector}[${index}]`));
        },

        /** 聚焦输入框、清空后逐字输入，走真实键盘事件以触发 onInput 防抖逻辑。 */
        async typeText(selector, text) {
            await clickPoint(await waitForPoint(() => centerOf(selector), selector));
            await evalJS(`
                const el = document.querySelector(${JSON.stringify(selector)});
                el.focus();
                if (el.value) {
                    el.value = '';
                    el.dispatchEvent(new Event('input', { bubbles: true }));
                }
                return true;
            `);
            for (const ch of text) {
                await session.send('Input.insertText', { text: ch });
                await session.send('Input.dispatchKeyEvent', { type: 'keyUp', key: ch });
            }
        },

        async pressKey(key) {
            const code = key === 'Escape' ? 27 : key === 'Enter' ? 13 : 0;
            for (const type of ['keyDown', 'keyUp']) {
                await session.send('Input.dispatchKeyEvent', {
                    type, key, code: key, windowsVirtualKeyCode: code, nativeVirtualKeyCode: code,
                });
            }
        },

        /** <select> 的原生下拉在无头模式下不可交互，直接赋值 + 派发 change。 */
        async selectValue(selector, value) {
            await evalJS(`
                const el = document.querySelector(${JSON.stringify(selector)});
                el.value = ${JSON.stringify(value)};
                el.dispatchEvent(new Event('change', { bubbles: true }));
                return true;
            `);
        },

        /** 分页跳转框：输入页码后回车。 */
        async jumpToPage(page) {
            const selector = '.card > .pagination .jumper .input';
            await clickPoint(await waitForPoint(() => centerOf(selector), selector));
            await session.send('Input.insertText', { text: String(page) });
            await this.pressKey('Enter');
        },
    };
}
