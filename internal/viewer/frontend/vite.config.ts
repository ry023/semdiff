import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: path.resolve(import.meta.dirname, "../dist"),
    emptyOutDir: true,
    cssCodeSplit: false,
    minify: "oxc",
    lib: {
      entry: path.resolve(import.meta.dirname, "src/main.tsx"),
      formats: ["iife"],
      name: "SemdiffViewer",
      fileName: () => "viewer.js",
    },
    rollupOptions: {
      output: {
        assetFileNames: (asset) =>
          asset.names?.some((name) => name.endsWith(".css"))
            ? "viewer.css"
            : "[name][extname]",
      },
    },
  },
});
