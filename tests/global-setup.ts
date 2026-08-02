import { FullConfig } from '@playwright/test';

async function globalSetup(config: FullConfig) {
  // The app now creates its own temp directory and cleans it up
  // No manual cleanup needed here
  console.log('Global setup complete - app will handle temp directory creation');
}

export default globalSetup;
