import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// 构建产物输出到 Go 包目录 internal/web/dist，由 web.go 通过 go:embed 内嵌
export default defineConfig({
  plugins: [react()],
  base: '/',
  server: {
    // 开发模式代理到 Go 后端（metrics.webui 服务），否则同源 /api/* 与
    // /openpt-icon.svg 会命中 Vite 的 SPA 回退而拿不到真实数据。
    proxy: {
      '/api': 'http://127.0.0.1:9090',
      '/openpt-icon.svg': 'http://127.0.0.1:9090',
    },
  },
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1200,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('antd') || id.includes('@ant-design')) {
              return 'vendor-antd';
            }
            if (id.includes('react') || id.includes('dayjs')) {
              return 'vendor-core';
            }
          }
        },
      },
    },
  },
});
