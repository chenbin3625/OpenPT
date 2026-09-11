// 复制到剪贴板。navigator.clipboard 仅在安全上下文（https / localhost）可用，
// 而本面板常以 http 形式部署在内网，因此保留 execCommand 兜底路径。
export async function copyText(text) {
    const value = String(text ?? '');
    if (!value) return false;

    if (navigator.clipboard && window.isSecureContext) {
        try {
            await Promise.race([
                navigator.clipboard.writeText(value),
                new Promise((_, reject) => setTimeout(() => reject(new Error('clipboard timeout')), 600)),
            ]);
            return true;
        } catch {
            // 继续走兜底
        }
    }

    const ta = document.createElement('textarea');
    ta.value = value;
    ta.setAttribute('readonly', '');
    ta.style.cssText = 'position:fixed;top:-1000px;opacity:0;';
    document.body.appendChild(ta);
    ta.select();
    let ok = false;
    try {
        ok = document.execCommand('copy');
    } catch {
        ok = false;
    }
    document.body.removeChild(ta);
    return ok;
}
