import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    include: ['test/js/**/*.test.js'],
    coverage: {
      provider: 'v8',
      include: ['cmd/pvs-ui/static/js/**/*.js'],
    },
  },
});
