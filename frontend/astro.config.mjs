// @ts-check
import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';

// 纯静态输出：构建后 frontend/dist 会被复制到 backend/web/dist 并嵌入 Go 二进制。
export default defineConfig({
  output: 'static',
  build: {
    format: 'directory',
  },
  // Astro 7 默认改用 JSX 式空白处理，这里保持 HTML 感知行为，避免行内元素间空格丢失。
  compressHTML: true,
  vite: {
    plugins: [tailwindcss()],
  },
  // 开发时把 API 与上传文件代理到本地 Go 服务，保持与生产同源的行为。
  server: {
    port: 4321,
    proxy: {
      '/api': 'http://localhost:8080',
      '/uploads': 'http://localhost:8080',
    },
  },
});
