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
    // vite 8 (rolldown) dropped the bundled esbuild — its native oxc
    // minifier replaces minify: 'esbuild'.
    minify: 'oxc',
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
