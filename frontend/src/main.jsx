import React from 'react';
import { createRoot } from 'react-dom/client';
import { ConfigProvider, App as AntdApp } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import App from './App';
import { ThemeProvider, useAppTheme } from './ThemeContext';
import './index.css';

dayjs.locale('zh-cn');

function Root() {
    const { themeConfig } = useAppTheme();
    return (
        <ConfigProvider locale={zhCN} theme={themeConfig}>
            <AntdApp>
                <App />
            </AntdApp>
        </ConfigProvider>
    );
}

createRoot(document.getElementById('root')).render(
    <React.StrictMode>
        <ThemeProvider>
            <Root />
        </ThemeProvider>
    </React.StrictMode>
);
