import { useEffect, useId, useRef, useState } from 'react';
import { useI18n } from '../i18n';
import { ApiError, isAbort } from '../lib/api';
import {
  inspectReceipt, MAX_CONFIG_BYTES, MAX_RECEIPT_BYTES, readReviewFile,
  replayImported, reviewCopy,
} from '../lib/reviewArtifacts';
import type { ImportedFile, ImportedReceipt } from '../lib/reviewArtifacts';
import type { ReplayResult } from '../lib/types';

type Slot = 'before' | 'after' | 'receipt';
type Files = Record<Slot, ImportedFile | null>;
const emptyFiles = (): Files => ({ before: null, after: null, receipt: null });

// An independent review has its own immutable file snapshots and request token.
// It never changes the active workflow's accepted policy or candidate verdict.
export function OfflineReviewPanel() {
  const { locale } = useI18n();
  const copy = reviewCopy[locale];
  const id = useId();
  const [files, setFiles] = useState<Files>(emptyFiles);
  const [reading, setReading] = useState<Record<Slot, boolean>>({ before: false, after: false, receipt: false });
  const [fileErrors, setFileErrors] = useState<Partial<Record<Slot, 'file' | 'invalid'>>>({});
  const [info, setInfo] = useState<ImportedReceipt | null>(null);
  const [result, setResult] = useState<ReplayResult | null>(null);
  const [error, setError] = useState<'mismatch' | 'error' | null>(null);
  const [canceled, setCanceled] = useState(false);
  const [busy, setBusy] = useState(false);
  const revision = useRef(0);
  const fileRevision = useRef({ before: 0, after: 0, receipt: 0 });
  const abort = useRef<AbortController | null>(null);

  useEffect(() => () => {
    revision.current += 1;
    for (const slot of ['before', 'after', 'receipt'] as const) fileRevision.current[slot] += 1;
    abort.current?.abort();
  }, []);

  function invalidate() {
    revision.current += 1;
    abort.current?.abort();
    abort.current = null;
    setBusy(false);
    setResult(null);
    setError(null);
    setCanceled(false);
  }

  async function selectFile(slot: Slot, file: File | undefined) {
    invalidate();
    const token = ++fileRevision.current[slot];
    setFiles((current) => ({ ...current, [slot]: null }));
    setFileErrors((current) => ({ ...current, [slot]: undefined }));
    if (slot === 'receipt') setInfo(null);
    setReading((current) => ({ ...current, [slot]: Boolean(file) }));
    if (!file) return;
    try {
      const imported = await readReviewFile(file, slot === 'receipt' ? MAX_RECEIPT_BYTES : MAX_CONFIG_BYTES);
      if (fileRevision.current[slot] !== token) return;
      if (slot === 'receipt') {
        try { setInfo(inspectReceipt(imported.text)); } catch {
          setFileErrors((current) => ({ ...current, [slot]: 'invalid' }));
          return;
        }
      }
      setFiles((current) => ({ ...current, [slot]: imported }));
    } catch {
      if (fileRevision.current[slot] === token) setFileErrors((current) => ({ ...current, [slot]: 'file' }));
    } finally {
      if (fileRevision.current[slot] === token) setReading((current) => ({ ...current, [slot]: false }));
    }
  }

  async function replay() {
    if (!files.before || !files.after || !files.receipt || Object.values(reading).some(Boolean)) return;
    invalidate();
    const token = revision.current;
    const controller = new AbortController();
    abort.current = controller;
    setBusy(true);
    try {
      const response = await replayImported(files.before, files.after, files.receipt, controller.signal);
      if (revision.current !== token) return;
      setResult(response);
    } catch (cause) {
      if (revision.current !== token) return;
      if (isAbort(cause)) setCanceled(true);
      else setError(cause instanceof ApiError && cause.code === 'EVIDENCE_MISMATCH' ? 'mismatch' : 'error');
    } finally {
      if (revision.current === token) {
        abort.current = null;
        setBusy(false);
      }
    }
  }

  return (
    <section className="replay-tools" aria-labelledby={`${id}-title`}>
      <h2 id={`${id}-title`}>{copy.title}</h2>
      <p className="field-hint">{copy.help}</p>
      <div className="replay-tools__grid">
        {(['before', 'after', 'receipt'] as const).map((slot) => (
          <div className="field" key={slot}>
            <label htmlFor={`${id}-${slot}`}>{copy[slot]}</label>
            <input id={`${id}-${slot}`} type="file" accept={slot === 'receipt' ? '.json,application/json' : '.acl,.cfg,.conf,.txt,text/plain'}
              onChange={(event) => { void selectFile(slot, event.target.files?.[0]); }} />
            {reading[slot] && <p role="status">{copy.loading}</p>}
            {files[slot] && <p className="field-hint">{files[slot]?.name} · {files[slot]?.bytes.byteLength} bytes</p>}
            {fileErrors[slot] && <p role="alert">{copy[fileErrors[slot]!]}</p>}
          </div>
        ))}
      </div>
      {info && <div className="technical-details">
        <p>{copy.outcome}: <code>{info.originalOutcome}</code></p>
        <p>{copy.hash}: <code className="hash-value">{info.sha256}</code></p>
      </div>}
      <p role="status" aria-live="polite">
        {busy ? copy.busy : error ? copy[error] : canceled ? copy.canceled : result
          ? result.status === 'TIMING_SENSITIVE' ? copy.timing : copy.matched : copy.unchecked}
        {result && <> · <code>{result.status}</code></>}
      </p>
      <button type="button" className="button button--secondary" disabled={busy || !files.before || !files.after || !files.receipt || Object.values(reading).some(Boolean)}
        onClick={() => { void replay(); }}>{copy.run}</button>
      {busy && <button type="button" className="button button--quiet" onClick={() => { invalidate(); setCanceled(true); }}>{copy.cancel}</button>}
    </section>
  );
}
