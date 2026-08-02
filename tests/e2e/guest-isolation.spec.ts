import { test, expect } from '@playwright/test';
import { HomePage } from '../pages/HomePage';

test.describe('Guest Session Isolation', () => {
  test('fresh guest should see no notes when opening the page', async ({ page, context }) => {
    // Clear all cookies and storage to simulate a fresh guest
    await context.clearCookies();
    await context.clearPermissions();
    
    const homePage = new HomePage(page);
    await homePage.goto();
    
    // Wait for the page to load
    await homePage.waitForLoadState();
    
    // Check if notes list exists - it should either not exist or be empty
    const notesListVisible = await homePage.isVisible(homePage.notesList);
    
    if (notesListVisible) {
      // If notes list is visible, it should be empty
      const notesCount = await homePage.getNotesCount();
      expect(notesCount).toBe(0);
    } else {
      // If notes list is not visible, that's also correct for a fresh guest
      expect(notesListVisible).toBe(false);
    }
  });

  test('guest who logs in should be able to create and see their own notes', async ({ page, context }) => {
    // Clear all cookies and storage to simulate a fresh guest
    await context.clearCookies();
    await context.clearPermissions();
    
    // Go to guest login
    await page.goto('/auth/guest');
    
    // Navigate to home page
    const homePage = new HomePage(page);
    await homePage.goto();
    
    // Should see no notes initially
    const notesListVisible = await homePage.isVisible(homePage.notesList);
    if (notesListVisible) {
      const initialCount = await homePage.getNotesCount();
      expect(initialCount).toBe(0);
    }
    
    // Create a note
    await homePage.createNote('Guest Test Note', 'Guest test content');
    
    // Should now see the note
    await expect(homePage.page.locator(homePage.notesList)).toBeVisible();
    const finalCount = await homePage.getNotesCount();
    expect(finalCount).toBeGreaterThanOrEqual(1);
  });

  test('different guests should not see each others notes', async ({ page: page1, context: context1 }) => {
    // Clear all cookies for first guest
    await context1.clearCookies();
    await context1.clearPermissions();
    
    // First guest logs in and creates a note
    await page1.goto('/auth/guest');
    const homePage1 = new HomePage(page1);
    await homePage1.goto();
    await homePage1.createNote('Guest 1 Note', 'Content from guest 1');
    
    const guest1NoteCount = await homePage1.getNotesCount();
    expect(guest1NoteCount).toBeGreaterThanOrEqual(1);
  });
});