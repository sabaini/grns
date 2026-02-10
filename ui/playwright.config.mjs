import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  use: {
    baseURL: 'http://127.0.0.1:7333',
    trace: 'retain-on-failure',
  },
  webServer: {
    command:
      "bash -lc 'tmpdir=$(mktemp -d); export GRNS_DB=\"$tmpdir/grns.db\"; export GRNS_API_URL=http://127.0.0.1:7333; go run ../cmd/grns srv'",
    url: 'http://127.0.0.1:7333/health',
    reuseExistingServer: true,
    timeout: 60_000,
  },
});
