// 极简 DOM 构建工具：替代 JSX，保持声明式写法但不引入运行时依赖。

// 这些键在 DOM 上必须按属性（property）赋值，setAttribute 语义不同或无效。
const ATTR_AS_PROP = new Set(['value', 'checked', 'selected', 'disabled', 'textContent']);

function applyProps(el, props) {
    for (const [key, value] of Object.entries(props)) {
        if (value === null || value === undefined || value === false) continue;

        if (key === 'class' || key === 'className') {
            el.className = Array.isArray(value) ? value.filter(Boolean).join(' ') : String(value);
        } else if (key === 'style' && typeof value === 'object') {
            Object.assign(el.style, value);
        } else if (key === 'dataset') {
            Object.assign(el.dataset, value);
        } else if (key === 'text') {
            el.textContent = String(value);
        } else if (key === 'html') {
            // 仅用于内置的静态 SVG 图标字符串，不接受外部数据
            el.innerHTML = value;
        } else if (key.startsWith('on') && typeof value === 'function') {
            el.addEventListener(key.slice(2).toLowerCase(), value);
        } else if (ATTR_AS_PROP.has(key)) {
            el[key] = value;
        } else {
            el.setAttribute(key, value === true ? '' : String(value));
        }
    }
}

function appendChildren(el, children) {
    for (const child of children) {
        if (child === null || child === undefined || child === false || child === '') continue;
        if (Array.isArray(child)) {
            appendChildren(el, child);
        } else if (child instanceof Node) {
            el.appendChild(child);
        } else {
            el.appendChild(document.createTextNode(String(child)));
        }
    }
}

// h('div', { class: 'x' }, '文本', h('span', null, '子节点'))
// 第二个参数可以直接省略成子节点（当它不是纯对象时）。
export function h(tag, props, ...children) {
    const el = document.createElement(tag);
    const isProps = props && typeof props === 'object' && !Array.isArray(props) && !(props instanceof Node);
    if (isProps) applyProps(el, props);
    appendChildren(el, isProps ? children : [props, ...children]);
    return el;
}

export function clearNode(el) {
    while (el.firstChild) el.removeChild(el.firstChild);
}
