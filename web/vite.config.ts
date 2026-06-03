import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [
    react(),
    // 给入口 module 脚本打 data-cfasync="false"，让 Cloudflare Rocket Loader 完全
    // 忽略它（避免它重排/包裹脚本导致 SPA 挂载时序异常）。用 order:"post" 在 Vite
    // 注入产物 script 之后再改，否则属性会被 Vite 重写覆盖。
    {
      name: "cf-rocket-loader-optout",
      transformIndexHtml: {
        order: "post",
        handler: (html: string) =>
          html.replace(/<script type="module"/g, '<script type="module" data-cfasync="false"'),
      },
    },
  ],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  build: { outDir: "dist" },
  server: {
    port: 5173,
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true },
    },
  },
});
