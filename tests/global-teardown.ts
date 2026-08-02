import { FullConfig } from '@playwright/test';

async function globalTeardown(config: FullConfig) {
  // The app now cleans up its own temp directory on shutdown
  // No manual cleanup needed here
  console.log('Global teardown complete - app handled temp directory cleanup');
}

export default globalTeardown;
