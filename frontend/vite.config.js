import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// 构建产物输出到 Go 包目录 internal/web/dist，由 web.go 通过 go:embed 内嵌
export default defineConfig({
  plugins: [react()],
  base: '/',
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
});
