import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  // A local .env (copied from .env.example) sets VITE_API_URL; unit tests must not see it
  envDir: mode === 'test' ? false : undefined,
  server: { port: Number(process.env['PORT'] ?? 7300), strictPort: true },
  build: { outDir: 'build' },
  test: {
    environment: 'jsdom',
    setupFiles: './src/setupTests.ts',
    // Playwright E2E tests live in e2e/ and are run separately
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.{test,spec}.{ts,tsx}', 'src/setupTests.ts', 'src/types/generated/**'],
      reporter: ['text-summary', 'json-summary', 'html'],
      reportsDirectory: 'coverage',
      // Current values rounded down. Raise them when coverage rises; never lower them.
      thresholds: { statements: 30, branches: 28, functions: 25, lines: 31 },
    },
  },
}));
