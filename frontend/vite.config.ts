import { defineConfig } from "vite";
import wails from "@wailsio/runtime/plugins/vite";
import { resolve } from "node:path";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  build: {
    assetsInlineLimit: 1_000_000,
    rolldownOptions: {
      input: {
        main: resolve(process.cwd(), "index.html"),
        tabs: resolve(process.cwd(), "tabs.html"),
        themes: resolve(process.cwd(), "themes.html"),
        plugins: resolve(process.cwd(), "plugins.html"),
      },
    },
  },
  plugins: [wails("./bindings")],
});
