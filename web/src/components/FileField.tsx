import { useId } from 'react';
import { Icon } from './Icon';

type FileLike = { name: string; bytes: { readonly byteLength: number } } | null;

// A palette-synchronized replacement for native file inputs: the real input
// stays visually hidden and label-associated, so activation and keyboard
// behavior are unchanged while the visible control matches the design system.
export function FileField({
  id,
  accept,
  chooseLabel,
  emptyLabel,
  file,
  onFile,
  disabled = false,
  ariaLabel,
}: {
  id?: string;
  accept?: string;
  chooseLabel: string;
  emptyLabel: string;
  file: FileLike;
  onFile: (file: File | undefined) => void;
  disabled?: boolean;
  ariaLabel?: string;
}) {
  const autoId = useId();
  const inputId = id ?? autoId;
  return (
    <div className="file-field">
      <label htmlFor={inputId} className="button button--secondary button--small file-field__choose">
        {chooseLabel}
      </label>
      <input
        id={inputId}
        type="file"
        className="sr-only"
        accept={accept}
        disabled={disabled}
        aria-label={ariaLabel}
        onChange={(event) => {
          onFile(event.target.files?.[0]);
          event.target.value = '';
        }}
      />
      <span className={`file-field__name${file ? '' : ' file-field__name--empty'}`} title={file?.name}>
        {file ? (
          <>
            <Icon name="file" size={15} className="file-field__icon" />
            <span className="file-field__filename">{file.name}</span>
            <span className="file-field__size">{formatBytes(file.bytes.byteLength)}</span>
          </>
        ) : (
          emptyLabel
        )}
      </span>
    </div>
  );
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const kib = bytes / 1024;
  if (kib < 1024) return `${kib.toFixed(kib >= 100 ? 0 : 1)} KiB`;
  const mib = kib / 1024;
  return `${mib.toFixed(mib >= 100 ? 0 : 1)} MiB`;
}
