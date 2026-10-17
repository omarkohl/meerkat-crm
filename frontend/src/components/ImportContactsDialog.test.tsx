import { describe, test, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, within } from '@testing-library/react';
import i18n from '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import ImportContactsDialog from './ImportContactsDialog';
import * as importApi from '../api/import';

vi.mock('../api/import', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/import')>()),
  uploadVCFForImport: vi.fn(),
  confirmVCFImport: vi.fn(),
}));

const row = (row_index: number, firstname: string, extra: Partial<importApi.ImportRowPreview> = {}) => ({
  row_index,
  parsed_contact: { firstname },
  validation_errors: [],
  duplicate_match: null,
  suggested_action: 'add' as const,
  ...extra,
});

const uploadVCF = async () => {
  vi.mocked(importApi.uploadVCFForImport).mockResolvedValue({
    session_id: 's',
    rows: [
      row(0, 'Carol'),
      row(1, 'Nameless', { validation_errors: ['First name is required'], suggested_action: 'skip' }),
      row(2, 'Bob', { warnings: ['Invalid phone "*100#" not imported'] }),
    ],
    total_rows: 3,
    valid_rows: 2,
    duplicate_count: 0,
    error_count: 1,
  });
  render(
    <SnackbarProvider>
      <ImportContactsDialog open onClose={() => {}} onImportComplete={() => {}} />
    </SnackbarProvider>,
  );
  const input = document.getElementById('import-file-input') as HTMLInputElement;
  fireEvent.change(input, { target: { files: [new File(['x'], 'contacts.vcf')] } });
  await screen.findByText('Carol');
};

const shownNames = () =>
  screen
    .getAllByRole('row')
    .slice(1)
    .map((r) => within(r).getAllByRole('cell')[1]?.textContent);

describe('ImportContactsDialog preview', { timeout: 20_000 }, () => {
  beforeEach(async () => {
    vi.resetAllMocks();
    await i18n.changeLanguage('en');
  });

  test('a summary chip filters the rows and a second click shows all again', async () => {
    await uploadVCF();
    const errors = screen.getByRole('button', { name: '1 with errors' });

    fireEvent.click(errors);
    expect(shownNames()).toEqual(['Nameless']);
    expect(screen.getByText('Showing 1 of 3 rows')).toBeInTheDocument();

    fireEvent.click(errors);
    expect(shownNames()).toEqual(['Carol', 'Nameless', 'Bob']);
  });

  test('a filtered row stays visible after its action changes', async () => {
    await uploadVCF();
    fireEvent.click(screen.getByRole('button', { name: '1 with warnings' }));
    expect(shownNames()).toEqual(['Bob']);

    // Filter on "to create", then skip all shown rows: they no longer match, but stay shown.
    fireEvent.click(screen.getByRole('button', { name: '2 to create' }));
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Set action for all shown rows' }));
    fireEvent.click(within(screen.getByRole('listbox')).getByText('Skip'));

    expect(shownNames()).toEqual(['Carol', 'Bob']);
    expect(screen.getByRole('button', { name: '0 to create' })).toBeInTheDocument();
  });

  test('a column header sorts the rows', async () => {
    await uploadVCF();
    fireEvent.click(screen.getByRole('button', { name: 'First Name' }));
    expect(shownNames()).toEqual(['Bob', 'Carol', 'Nameless']);
    fireEvent.click(screen.getByRole('button', { name: 'First Name' }));
    expect(shownNames()).toEqual(['Nameless', 'Carol', 'Bob']);
  });

  test('the result names the skipped rows and why', async () => {
    await uploadVCF();
    vi.mocked(importApi.confirmVCFImport).mockResolvedValue({
      total_processed: 3,
      created: 1,
      updated: 0,
      skipped: 2,
      errors: [],
      skipped_rows: [
        { row: 2, name: '', reason: 'invalid', detail: 'First name is required' },
        { row: 3, name: 'Bob', reason: 'skipped' },
      ],
    });
    fireEvent.click(screen.getByRole('button', { name: 'Import' }));

    const table = await screen.findByRole('table', { name: 'Skipped rows' });
    const rows = within(table)
      .getAllByRole('row')
      .slice(1)
      .map((r) =>
        within(r)
          .getAllByRole('cell')
          .map((c) => c.textContent),
      );
    expect(rows).toEqual([
      ['2', '—', 'Invalid: First name is required'],
      ['3', 'Bob', 'Skipped in the review'],
    ]);
  });
});
