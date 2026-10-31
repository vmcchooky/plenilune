export function downloadText(filename: string, content: string, mime: string): void {
  downloadBytes(filename, new TextEncoder().encode(content), mime);
}

export function downloadBytes(filename: string, bytes: Uint8Array, mime: string): void {
  const url = URL.createObjectURL(new Blob([new Uint8Array(bytes).buffer], { type: mime }));
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}

export async function copyText(value: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(value);
    return true;
  } catch {
    return false;
  }
}
