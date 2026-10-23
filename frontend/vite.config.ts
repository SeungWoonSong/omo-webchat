import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import en from './src/i18n/locales/en.json';
import manifest from './public/manifest.json';

export default defineConfig({
  plugins: [react(), {
    name: 'localized-product-name',
    transformIndexHtml: (html) => html.replaceAll('%APP_NAME%', en['app.name']),
    generateBundle() {
      this.emitFile({
        type: 'asset',
        fileName: 'manifest.json',
        source: JSON.stringify({ ...manifest, name: en['app.name'], short_name: en['app.name'] }),
      });
    },
  }],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
});
