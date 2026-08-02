export const testConfig = {
  baseURL: process.env.BASE_URL || 'http://localhost:8080',
  timeout: 30000,
  retries: 2,
};

export const testNotes = {
  validNote: {
    title: 'Test Note',
    content: 'This is test content for the note',
  },
  emptyTitle: {
    title: '',
    content: 'This has content but no title',
  },
  emptyContent: {
    title: 'This has title but no content',
    content: '',
  },
  specialChars: {
    title: 'Note with special chars! @#$%',
    content: 'Content with \n newlines and special chars',
  },
};
