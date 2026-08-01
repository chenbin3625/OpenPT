import React from 'react';
import { createRoot } from 'react-dom/client';
import { ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import App from './App';
import './index.css';

dayjs.locale('zh-cn');

createRoot(document.getElementById('root')).render(
    <ConfigProvider
        locale={zhCN}
        theme={{
            token: {
                colorPrimary: '#2563eb',
                colorSuccess: '#047857',
                colorWarning: '#b45309',
                colorError: '#dc2626',
                borderRadius: 6,
                fontSize: 13,
            },
        }}
    >
        <App />
    </ConfigProvider>
);
