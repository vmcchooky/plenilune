import { useCallback, useEffect, useId, useRef, useState } from 'react';
import type { KeyboardEvent } from 'react';
import { Icon } from './Icon';

export type SelectOption = { value: string; label: string };

// A palette-synchronized listbox for the app's controlled dropdowns. Native
// <select> popups render with the OS theme, which cannot be styled; this
// keeps the same keyboard contract (arrows/Home/End/Escape/typeahead,
// Tab keeps moving focus) with focus retained on the trigger and
// aria-activedescendant pointing at the active option.
export function Select({
  id,
  value,
  onChange,
  options,
  disabled = false,
  ariaLabel,
  className = '',
}: {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  disabled?: boolean;
  ariaLabel?: string;
  className?: string;
}) {
  const listId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const typeahead = useRef({ buffer: '', timer: 0 });
  const [open, setOpen] = useState(false);
  const [above, setAbove] = useState(false);

  const selectedIndex = Math.max(0, options.findIndex((option) => option.value === value));
  const [activeIndex, setActiveIndex] = useState(selectedIndex);
  const selected = options[selectedIndex];

  const openList = useCallback((focusIndex?: number) => {
    setActiveIndex(focusIndex ?? selectedIndex);
    const rect = buttonRef.current?.getBoundingClientRect();
    if (rect) {
      const spaceBelow = window.innerHeight - rect.bottom;
      setAbove(spaceBelow < 240 && rect.top > 240);
    }
    setOpen(true);
  }, [selectedIndex]);

  const closeList = useCallback((focus = true) => {
    setOpen(false);
    if (focus) buttonRef.current?.focus();
  }, []);

  const commit = useCallback((index: number) => {
    const option = options[index];
    if (option) onChange(option.value);
    closeList();
  }, [options, onChange, closeList]);

  // Click outside closes without committing.
  useEffect(() => {
    if (!open) return;
    function onPointerDown(event: PointerEvent) {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    }
    document.addEventListener('pointerdown', onPointerDown, true);
    return () => document.removeEventListener('pointerdown', onPointerDown, true);
  }, [open]);

  // Keep the keyboard-active option visible while navigating.
  useEffect(() => {
    if (!open) return;
    const option = listRef.current?.children[activeIndex] as HTMLElement | undefined;
    option?.scrollIntoView?.({ block: 'nearest' });
  }, [open, activeIndex]);

  function typeaheadTo(char: string) {
    const state = typeahead.current;
    window.clearTimeout(state.timer);
    state.buffer = (state.buffer + char.toLocaleLowerCase()).slice(0, 24);
    state.timer = window.setTimeout(() => { state.buffer = ''; }, 600);
    const start = open ? activeIndex + 1 : 0;
    for (let offset = 0; offset < options.length; offset += 1) {
      const index = (start + offset) % options.length;
      if (options[index].label.toLocaleLowerCase().startsWith(state.buffer)) {
        if (open) setActiveIndex(index);
        else onChange(options[index].value);
        return;
      }
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const printable = event.key.length === 1 && !event.metaKey && !event.ctrlKey && !event.altKey;
    const last = options.length - 1;
    if (!open) {
      switch (event.key) {
        case 'ArrowDown': event.preventDefault(); openList(Math.min(selectedIndex + 1, last)); return;
        case 'ArrowUp': event.preventDefault(); openList(Math.max(selectedIndex - 1, 0)); return;
        case 'Enter':
        case ' ': event.preventDefault(); openList(); return;
        default:
          if (printable) { event.preventDefault(); typeaheadTo(event.key); }
          return;
      }
    }
    switch (event.key) {
      case 'ArrowDown': event.preventDefault(); setActiveIndex((index) => Math.min(index + 1, last)); return;
      case 'ArrowUp': event.preventDefault(); setActiveIndex((index) => Math.max(index - 1, 0)); return;
      case 'Home': event.preventDefault(); setActiveIndex(0); return;
      case 'End': event.preventDefault(); setActiveIndex(last); return;
      case 'Enter':
      case ' ': event.preventDefault(); commit(activeIndex); return;
      case 'Escape': event.preventDefault(); closeList(); return;
      case 'Tab': setOpen(false); return; // let focus move on; no commit
      default:
        if (printable) { event.preventDefault(); typeaheadTo(event.key); }
    }
  }

  return (
    <div ref={rootRef} className={`select ${className}`.trim()}>
      <button
        ref={buttonRef}
        id={id}
        type="button"
        className="select-trigger"
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-activedescendant={open ? `${listId}-${activeIndex}` : undefined}
        aria-label={ariaLabel}
        onClick={() => (open ? setOpen(false) : openList())}
        onKeyDown={onKeyDown}
      >
        <span className="select-trigger__label">{selected?.label}</span>
        <Icon name="chevronDown" size={16} className="select-trigger__chevron" />
      </button>
      {open && (
        <ul
          ref={listRef}
          id={listId}
          role="listbox"
          aria-labelledby={id}
          className={`select-list${above ? ' select-list--above' : ''}`}
        >
          {options.map((option, index) => {
            const isSelected = option.value === value;
            return (
              <li
                key={option.value}
                id={`${listId}-${index}`}
                role="option"
                aria-selected={isSelected}
                data-active={index === activeIndex || undefined}
                className="select-option"
                onPointerEnter={() => setActiveIndex(index)}
                onPointerDown={(event) => { event.preventDefault(); commit(index); }}
                onClick={() => commit(index)}
              >
                <span className="select-option__label">{option.label}</span>
                {isSelected && <Icon name="check" size={16} className="select-option__check" />}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
