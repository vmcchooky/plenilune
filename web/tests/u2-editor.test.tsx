import { cleanup, render, screen, fireEvent } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { LocaleProvider } from '../src/i18n';
import { SourceEditor } from '../src/components/SourceInputs';

afterEach(cleanup);

function renderEditor(ui: React.ReactElement) {
  return render(<LocaleProvider>{ui}</LocaleProvider>);
}

describe('U2 — SourceEditor soft-wrap & line numbers', () => {
  it('renders line numbers gutter matching content lines', () => {
    const value = 'line 1\nline 2\nline 3';
    renderEditor(
      <SourceEditor
        id="before"
        label="Cấu hình trước"
        value={value}
        help="Help text"
        disabled={false}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText('1')).toBeTruthy();
    expect(screen.getByText('2')).toBeTruthy();
    expect(screen.getByText('3')).toBeTruthy();
  });

  it('toggles wrap mode when wrap button is clicked', () => {
    const value = 'access-list 101 permit ip any any';
    renderEditor(
      <SourceEditor
        id="before"
        label="Cấu hình trước"
        value={value}
        help="Help text"
        disabled={false}
        onChange={vi.fn()}
      />
    );

    const textarea = screen.getByRole('textbox', { name: /Cấu hình trước/i });
    expect(textarea.getAttribute('wrap')).toBe('soft');

    const toggleBtn = screen.getByRole('button', { name: /ngắt dòng|wrap/i });
    fireEvent.click(toggleBtn);
    expect(textarea.getAttribute('wrap')).toBe('off');

    fireEvent.click(toggleBtn);
    expect(textarea.getAttribute('wrap')).toBe('soft');
  });

  it('shows ruler with max line length and warns for lines over 80 characters', () => {
    const longLine = 'ip access-list extended VERY_LONG_RULE_NAME permit tcp 192.168.100.0 0.0.0.255 host 10.0.0.1 eq 443';
    renderEditor(
      <SourceEditor
        id="after"
        label="Cấu hình sau"
        value={longLine}
        help="Help text"
        disabled={false}
        onChange={vi.fn()}
      />
    );

    expect(screen.getByText(/vượt 80|exceeds 80/i)).toBeTruthy();
  });
});
