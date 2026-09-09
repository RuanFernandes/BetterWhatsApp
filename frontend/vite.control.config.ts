import { defineConfig } from "vite";
import { resolve } from "node:path";

export default defineConfig({
  build: {
    outDir: resolve(process.cwd(), "dist/control"),
    emptyOutDir: true,
    assetsInlineLimit: 10_000_000,
    lib: {
      entry: resolve(process.cwd(), "src/main.ts"),
      name: "BetterWhatsAppControl",
      fileName: () => "control.js",
      formats: ["iife"],
      cssFileName: "control",
    },
  },
});
