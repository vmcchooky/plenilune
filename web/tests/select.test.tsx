import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Select } from '../src/components/Select';
import type { SelectOption } from '../src/components/Select';

const options: SelectOption[] = [
  { value: 'acl', label: 'ACL' },
  { value: 'prefix-list', label: 'Prefix list' },
  { value: 'standard', label: 'Standard ACL' },
];

afterEach(cleanup);

describe('Select', () => {
  it('opens on click, picks an option and returns focus to the trigger', () => {
    const onChange = vi.fn();
    render(<Select id="scope" value="acl" onChange={onChange} options={options} ariaLabel="Scope" />);
    const trigger = screen.getByRole('button', { name: 'Scope' });
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    fireEvent.click(trigger);
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(screen.getByRole('listbox')).toBeTruthy();
    fireEvent.click(screen.getByRole('option', { name: 'Prefix list' }));
    expect(onChange).toHaveBeenCalledWith('prefix-list');
    expect(screen.queryByRole('listbox')).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it('keyboard: arrows move, Enter commits, Escape closes without committing', () => {
    const onChange = vi.fn();
    render(<Select id="scope" value="acl" onChange={onChange} options={options} ariaLabel="Scope" />);
    const trigger = screen.getByRole('button', { name: 'Scope' });
    fireEvent.keyDown(trigger, { key: 'ArrowDown' });
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    expect(trigger.getAttribute('aria-activedescendant')).toContain('-1'); // opens on the next option
    fireEvent.keyDown(trigger, { key: 'Enter' });
    expect(onChange).toHaveBeenCalledWith('prefix-list');
    expect(screen.queryByRole('listbox')).toBeNull();
    fireEvent.keyDown(trigger, { key: 'ArrowUp' });
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
    fireEvent.keyDown(trigger, { key: 'Escape' });
    expect(screen.queryByRole('listbox')).toBeNull();
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it('typeahead selects the matching option from the closed state', () => {
    const onChange = vi.fn();
    render(<Select id="scope" value="acl" onChange={onChange} options={options} ariaLabel="Scope" />);
    const trigger = screen.getByRole('button', { name: 'Scope' });
    fireEvent.keyDown(trigger, { key: 'p' });
    expect(onChange).toHaveBeenCalledWith('prefix-list');
  });

  it('exposes the active option through aria-activedescendant while open', () => {
    render(<Select id="scope" value="prefix-list" onChange={() => {}} options={options} ariaLabel="Scope" />);
    const trigger = screen.getByRole('button', { name: 'Scope' });
    fireEvent.click(trigger);
    const active = trigger.getAttribute('aria-activedescendant');
    expect(active).toBeTruthy();
    expect(document.getElementById(active!)?.textContent).toBe('Prefix list');
    expect(screen.getByRole('option', { name: 'Prefix list' }).getAttribute('aria-selected')).toBe('true');
  });

  it('does not open when disabled', () => {
    render(<Select id="scope" value="acl" onChange={() => {}} options={options} ariaLabel="Scope" disabled />);
    const trigger = screen.getByRole('button', { name: 'Scope' }) as HTMLButtonElement;
    expect(trigger.disabled).toBe(true);
    fireEvent.click(trigger);
    expect(screen.queryByRole('listbox')).toBeNull();
  });
});
