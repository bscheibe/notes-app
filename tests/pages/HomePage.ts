import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class HomePage extends BasePage {
  readonly titleInput = 'input[name="title"]';
  readonly contentTextarea = 'textarea[name="content"]';
  readonly saveButton = 'button[type="submit"]';
  readonly notesList = '.notes-list ul';
  readonly noteLinks = '.notes-list a';
  readonly message = '.message';

  constructor(page: Page) {
    super(page);
  }

  async goto() {
    await super.goto('/');
    await this.waitForLoadState();
  }

  async createNote(title: string, content: string) {
    await this.fill(this.titleInput, title);
    await this.fill(this.contentTextarea, content);
    await this.click(this.saveButton);
    await this.waitForLoadState();
  }

  async getNotesCount(): Promise<number> {
    await this.page.waitForSelector(this.notesList, { state: 'visible' });
    const notes = await this.page.locator(this.noteLinks).all();
    return notes.length;
  }

  async getNoteTitles(): Promise<string[]> {
    await this.page.waitForSelector(this.notesList, { state: 'visible' });
    return await this.page.locator(this.noteLinks).allTextContents();
  }

  async hasNote(title: string): Promise<boolean> {
    const titles = await this.getNoteTitles();
    return titles.some(t => t.includes(title));
  }

  async getErrorMessage(): Promise<string> {
    if (await this.isVisible(this.message)) {
      return await this.getText(this.message);
    }
    return '';
  }

  async assertNoteCreated(title: string) {
    await expect(this.page.locator(this.notesList)).toBeVisible();
    const hasNote = await this.hasNote(title);
    expect(hasNote).toBe(true);
  }
}
