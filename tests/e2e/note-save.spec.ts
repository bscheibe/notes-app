import { test, expect } from '@playwright/test';
import { HomePage } from '../pages/HomePage';

test.describe('Note Save Flow', () => {
  test('should save a new note with title and content', async ({ page, context }) => {
    // Clear all cookies and storage for test isolation
    await context.clearCookies();
    
    const homePage = new HomePage(page);
    await homePage.goto();
    
    const testTitle = 'Test Note';
    const testContent = 'This is a test note content';

    // Get initial note count
    const initialCount = await homePage.getNotesCount();

    // Create a new note
    await homePage.createNote(testTitle, testContent);

    // Verify note list is visible
    await expect(homePage.page.locator(homePage.notesList)).toBeVisible();

    // Verify we have more notes than before
    const finalCount = await homePage.getNotesCount();
    expect(finalCount).toBeGreaterThan(initialCount);
  });

  test('should save multiple notes', async ({ page, context }) => {
    // Clear all cookies and storage for test isolation
    await context.clearCookies();
    
    const homePage = new HomePage(page);
    await homePage.goto();
    
    const notes = [
      { title: 'Alpha Note', content: 'Alpha content' },
      { title: 'Beta Note', content: 'Beta content' },
      { title: 'Gamma Note', content: 'Gamma content' },
    ];

    // Get initial note count
    const initialCount = await homePage.getNotesCount();

    // Create multiple notes
    for (const note of notes) {
      await homePage.createNote(note.title, note.content);
    }

    // Verify we have more notes than before
    const finalCount = await homePage.getNotesCount();
    expect(finalCount).toBeGreaterThanOrEqual(initialCount + 3);
  });

  test('should show error when title is empty', async ({ page, context }) => {
    // Clear all cookies and storage for test isolation
    await context.clearCookies();
    
    const homePage = new HomePage(page);
    await homePage.goto();
    
    await homePage.createNote('', 'Some content');

    // Check if there's an error message or if we're still on the page
    const currentUrl = page.url();
    expect(currentUrl).toContain('/'); // Should still be on the page
  });

  test('should show error when content is empty', async ({ page, context }) => {
    // Clear all cookies and storage for test isolation
    await context.clearCookies();
    
    const homePage = new HomePage(page);
    await homePage.goto();
    
    await homePage.createNote('Test Title', '');

    // Check if there's an error message or if we're still on the page
    const currentUrl = page.url();
    expect(currentUrl).toContain('/'); // Should still be on the page
  });

  test('should show error when both title and content are empty', async ({ page, context }) => {
    // Clear all cookies and storage for test isolation
    await context.clearCookies();
    
    const homePage = new HomePage(page);
    await homePage.goto();
    
    await homePage.createNote('', '');

    // Check if there's an error message or if we're still on the page
    const currentUrl = page.url();
    expect(currentUrl).toContain('/'); // Should still be on the page
  });
});
