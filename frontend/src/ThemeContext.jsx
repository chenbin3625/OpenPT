import React, { createContext, useContext, useEffect, useState, useMemo } from 'react';
import { theme as antdTheme } from 'antd';

const ThemeContext = createContext({
    mode: 'auto',
    isDark: false,
    setMode: () => {},
    toggleMode: () => {},
});

export function ThemeProvider({ children }) {
    const [mode, setMode] = useState(() => {
        return localStorage.getItem('openpt-theme-mode') || 'auto';
    });

    const [systemDark, setSystemDark] = useState(() => {
        return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
    });

    useEffect(() => {
        if (!window.matchMedia) return;
        const media = window.matchMedia('(prefers-color-scheme: dark)');
        const listener = e => setSystemDark(e.matches);
        media.addEventListener('change', listener);
        return () => media.removeEventListener('change', listener);
    }, []);

    const isDark = useMemo(() => {
        if (mode === 'dark') return true;
        if (mode === 'light') return false;
        return systemDark;
    }, [mode, systemDark]);

    const handleSetMode = newMode => {
        setMode(newMode);
        localStorage.setItem('openpt-theme-mode', newMode);
    };

    const toggleMode = () => {
        handleSetMode(isDark ? 'light' : 'dark');
    };

    useEffect(() => {
        if (isDark) {
            document.documentElement.classList.add('dark');
            document.documentElement.setAttribute('data-theme', 'dark');
        } else {
            document.documentElement.classList.remove('dark');
            document.documentElement.setAttribute('data-theme', 'light');
        }
    }, [isDark]);

    const themeConfig = useMemo(() => ({
        algorithm: isDark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: {
            colorPrimary: '#2563eb',
            colorSuccess: '#10b981',
            colorWarning: '#f59e0b',
            colorError: '#ef4444',
            colorInfo: '#3b82f6',
            borderRadius: 8,
            fontSize: 13,
            fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
            colorBgContainer: isDark ? '#1e293b' : '#ffffff',
            colorBgLayout: isDark ? '#0f172a' : '#f8fafc',
            colorBgElevated: isDark ? '#1e293b' : '#ffffff',
            colorBorderSecondary: isDark ? '#334155' : '#e2e8f0',
        },
        components: {
            Card: {
                colorBgContainer: isDark ? '#1e293b' : '#ffffff',
                boxShadowTertiary: isDark ? '0 1px 3px 0 rgba(0, 0, 0, 0.37)' : '0 1px 3px 0 rgba(0, 0, 0, 0.05)',
            },
            Table: {
                colorBgContainer: isDark ? '#1e293b' : '#ffffff',
                headerBg: isDark ? '#0f172a' : '#f8fafc',
                rowHoverBg: isDark ? '#334155' : '#f1f5f9',
            },
            Segmented: {
                trackBg: isDark ? '#0f172a' : '#f1f5f9',
            },
        },
    }), [isDark]);

    return (
        <ThemeContext.Provider value={{ mode, isDark, setMode: handleSetMode, toggleMode, themeConfig }}>
            {children}
        </ThemeContext.Provider>
    );
}

export function useAppTheme() {
    return useContext(ThemeContext);
}
