// 主题控制：auto 跟随系统，light / dark 为显式选择，选择结果持久化到 localStorage。
const THEME_STORAGE_KEY = 'openpt-theme-mode';

export function createThemeController() {
    const media = window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null;
    const listeners = new Set();

    let mode = 'auto';
    try {
        const saved = localStorage.getItem(THEME_STORAGE_KEY);
        if (saved === 'light' || saved === 'dark' || saved === 'auto') mode = saved;
    } catch {
        // 隐私模式下 localStorage 可能不可用，退回默认值
    }

    const isDark = () => (mode === 'auto' ? Boolean(media && media.matches) : mode === 'dark');

    const apply = () => {
        const dark = isDark();
        document.documentElement.dataset.theme = dark ? 'dark' : 'light';
        for (const fn of listeners) fn(dark, mode);
    };

    if (media) {
        media.addEventListener('change', () => {
            if (mode === 'auto') apply();
        });
    }

    const setMode = next => {
        mode = next;
        try {
            localStorage.setItem(THEME_STORAGE_KEY, next);
        } catch {
            // 忽略写入失败，仅本次会话生效
        }
        apply();
    };

    apply();

    return {
        get mode() {
            return mode;
        },
        get isDark() {
            return isDark();
        },
        setMode,
        toggle: () => setMode(isDark() ? 'light' : 'dark'),
        subscribe(fn) {
            listeners.add(fn);
            fn(isDark(), mode);
            return () => listeners.delete(fn);
        },
    };
}
