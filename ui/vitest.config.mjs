import { defineConfig } from 'vitest/config';

export default defineConfig({
  esbuild: {
    jsx: 'automatic',
    jsxImportSource: 'preact',
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./test/setup.js'],
    globals: true,
    css: false,
    include: ['src/**/*.test.{js,jsx}'],
    exclude: ['e2e/**'],
  },
});
