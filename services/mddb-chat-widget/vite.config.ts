import { defineConfig } from 'vite';
import { resolve } from 'path';

export default defineConfig({
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      name: 'MddbChat',
      fileName: 'mddb-chat',
      formats: ['iife'],
    },
    outDir: 'dist',
    minify: 'esbuild',
    rollupOptions: {
      output: {
        entryFileNames: 'mddb-chat.min.js',
        inlineDynamicImports: true,
      },
    },
  },
  server: {
    port: 11032,
  },
});
