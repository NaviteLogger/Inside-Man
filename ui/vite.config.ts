// defineConfig comes from vitest/config so the test block type-checks.
// Vitest 4 stopped augmenting Vite's own UserConfig with a test key.
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    // The BFF serves the API in development, so the UI never needs CORS.
    proxy: {
      '/api': { target: process.env.BFF_URL ?? 'http://localhost:8080', changeOrigin: true },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
  },
});
