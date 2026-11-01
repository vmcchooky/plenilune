import { useEffect, useRef, useState } from 'react';
import { useI18n } from '../i18n';
import type { Review } from '../hooks/useReview';
import { actionLabel, sourceLine } from '../lib/conditions';
import { stopReasonLabel } from '../lib/labels';
import type { BoxText, ChangeClass, DecidingRule, Packet } from '../lib/types';
import { PartialNotice } from './PartialNotice';
import { BusyPanel, SectionHeading, StatusMark, TechnicalDetails } from './Ui';

function protoName(n: number): string {
  return n === 6 ? 'TCP' : n === 17 ? 'UDP' : n === 1 ? 'ICMP' : String(n);
}

export function packetText(p: Packet): string {
  const ports = p.protocol === 6 || p.protocol === 17;
  return `${protoName(p.protocol)} ${p.src_ip}${ports ? `:${p.src_port}` : ''} → ${p.dst_ip}${ports ? `:${p.dst_port}` : ''}`;
}

function boxText(b: BoxText, any: string): string {
  const port = (v: string) => (v === 'any' ? any : v);
  const ports = b.dst_ports === 'n/a' ? '' : ` · src ${port(b.src_ports)} · dst ${port(b.dst_ports)}`;
  const src = b.src === 'any' ? any : b.src;
  const dst = b.dst === 'any' ? any : b.dst;
  return `${b.protocol} ${src} → ${dst}${ports}`;
}

function RuleLine({ rule, source, firstLine }: { rule: DecidingRule; source: string; firstLine: number }) {
  const { t } = useI18n();
  if (rule.implicit) return <span>{actionLabel(t, 'deny')} · {t('changes.implicit')}</span>;
  const text = sourceLine(source, rule.line)?.trim();
  const configLine = (rule.line ?? 0) + firstLine - 1;
  return (
    <span>
      {actionLabel(t, rule.action)} · {t('changes.rule', { line: configLine })}
      {text && <> — <code>{text}</code></>}
    </span>
  );
}

function ClassRow({ c, review, onProbe }: { c: ChangeClass; review: Review; onProbe: (p: Packet) => void }) {
  const { t } = useI18n();
  const source = review.accepted ?? review.parsed;
  if (!source) return null;
  const first = review.parsed?.firstLine ?? { before: 1, after: 1 };
  return (
    <li className={`card change-class change-class--${c.effect}`}>
      <div className="change-class__head">
        <StatusMark tone={c.effect === 'newly_denied' ? 'bad' : 'warn'} label={t(`changes.effect.${c.effect}`)} />
        {c.examples[0] && <code className="change-class__region">{boxText(c.examples[0], t('changes.any'))}</code>}
        {c.scope !== 'undeclared' && <StatusMark tone={c.scope === 'inside_allowed' ? 'good' : 'bad'} label={t(`changes.scope.${c.scope}`)} />}
      </div>
      <dl className="change-class__rules">
        <div><dt>{t('changes.before')}</dt><dd><RuleLine rule={c.before} source={source.before} firstLine={first.before} /></dd></div>
        <div><dt>{t('changes.after')}</dt><dd><RuleLine rule={c.after} source={source.after} firstLine={first.after} /></dd></div>
        <div><dt>{t('changes.example')}</dt><dd><code>{packetText(c.witness.packet)}</code>
          <button type="button" className="button button--quiet button--small" onClick={() => onProbe(c.witness.packet)}>{t('changes.probe')}</button></dd></div>
        {c.outside_witness && <div><dt>{t('changes.outside')}</dt><dd><code>{packetText(c.outside_witness.packet)}</code></dd></div>}
      </dl>
      {(c.pieces > 1 || c.examples_truncated) && (
        <TechnicalDetails title={t('changes.region')}>
          <ul>{c.examples.map((b, i) => <li key={i}><code>{boxText(b, t('changes.any'))}</code></li>)}</ul>
          {c.examples_truncated && <p className="field-hint">{t('changes.moreRegions', { count: c.pieces })}</p>}
        </TechnicalDetails>
      )}
    </li>
  );
}

export function ChangeClasses({ review }: { review: Review }) {
  const { t } = useI18n();
  const [probeSeed, setProbeSeed] = useState<Packet | null>(null);
  const [probeRequested, setProbeRequested] = useState(0);
  const report = review.aclChanges;
  if (!report) return null;
  const denied = report.classes.filter((c) => c.effect === 'newly_denied').length;
  const permitted = report.classes.length - denied;
  const classList = (
    <ol className="change-class-list">
      {report.classes.map((c, i) => <ClassRow key={i} c={c} review={review} onProbe={(p) => { setProbeSeed(p); setProbeRequested((n) => n + 1); }} />)}
    </ol>
  );
  return (
    <section className="change-classes" aria-labelledby="changes-title">
      <SectionHeading id="changes-title" title={t('changes.title')} note={t('changes.help')} />
      {!report.complete && report.partial ? (
        // P1-2: the decided classes are real and stay listed; the partial
        // notice bounds what was never decided. Pre-v0.4 artifacts carry
        // no partial and keep the notice-only rendering below.
        <>
          <p className="banner banner--warn">{t('changes.incomplete', { reason: stopReasonLabel(t, report.reason ?? '') })}</p>
          {report.classes.length > 0 && (
            <>
              <p className="field-hint">{t('partial.classesNote')}</p>
              {classList}
            </>
          )}
          <PartialNotice partial={report.partial} showDecidedPairs={false} showUndecidedPairs />
        </>
      ) : !report.complete ? (
        <p className="banner banner--warn">{t('changes.incomplete', { reason: stopReasonLabel(t, report.reason ?? '') })}</p>
      ) : report.classes.length === 0 ? (
        <p className="banner banner--neutral">{t('changes.none')}</p>
      ) : (
        <>
          <p className="change-classes__summary">{t('changes.summary', { denied, permitted })}</p>
          {classList}
          {report.classes_truncated && <p className="field-hint">{t('changes.truncated', { shown: report.classes.length, total: report.total_classes })}</p>}
        </>
      )}
      <PacketProbe review={review} seed={probeSeed ?? report.classes[0]?.witness.packet ?? null} requested={probeRequested} />
    </section>
  );
}

function validIPv4(s: string): boolean {
  const parts = s.trim().split('.');
  return parts.length === 4 && parts.every((p) => /^\d{1,3}$/.test(p) && Number(p) <= 255);
}

export function PacketProbe({ review, seed, requested = 0 }: { review: Review; seed: Packet | null; requested?: number }) {
  const { t } = useI18n();
  const panel = useRef<HTMLDivElement>(null);
  const [form, setForm] = useState({ protocol: '6', src: '192.0.2.10', dst: '198.51.100.10', sport: '0', dport: '22' });
  const [invalid, setInvalid] = useState(false);
  useEffect(() => {
    if (seed) setForm({ protocol: String(seed.protocol), src: seed.src_ip, dst: seed.dst_ip, sport: String(seed.src_port), dport: String(seed.dst_port) });
  }, [seed]);
  // Opening is driven by an explicit "try this packet" action, then focus moves
  // to the first field so keyboard users land in the form.
  useEffect(() => {
    if (requested === 0 || !panel.current) return;
    const details = panel.current.querySelector('details');
    if (details) details.open = true;
    panel.current.scrollIntoView({ block: 'nearest' });
    panel.current.querySelector<HTMLInputElement>('#probe-protocol')?.focus();
  }, [requested]);
  const source = review.accepted ?? review.parsed;
  if (!source) return null;
  const first = review.parsed?.firstLine ?? { before: 1, after: 1 };
  const result = review.probeResult;

  function run() {
    const protocol = Number(form.protocol);
    const sport = Number(form.sport);
    const dport = Number(form.dport);
    const ports = protocol === 6 || protocol === 17;
    const ok = Number.isInteger(protocol) && protocol >= 0 && protocol <= 255 && validIPv4(form.src) && validIPv4(form.dst)
      && Number.isInteger(sport) && Number.isInteger(dport) && sport >= 0 && dport >= 0 && sport <= 65535 && dport <= 65535
      && (ports || (sport === 0 && dport === 0));
    setInvalid(!ok);
    if (ok) void review.probe({ protocol, src_ip: form.src.trim(), dst_ip: form.dst.trim(), src_port: sport, dst_port: dport });
  }

  const row = (label: string, d: NonNullable<typeof result>['before'], text: string, firstLine: number) => (
    <div><dt>{label}</dt><dd>
      <StatusMark tone={d.action === 'permit' ? 'good' : 'bad'} label={actionLabel(t, d.action)} />
      {d.implicit ? <span> · {t('changes.implicit')}</span> : <span> · {t('changes.rule', { line: (d.line ?? 0) + firstLine - 1 })} — <code>{sourceLine(text, d.line)?.trim()}</code></span>}
    </dd></div>
  );

  return (
    <div ref={panel}>
    <TechnicalDetails title={t('probe.title')} defaultOpen={Boolean(result)}>
      <p className="field-hint">{t('probe.help')}</p>
      <form className="probe-form" onSubmit={(e) => { e.preventDefault(); run(); }}>
        {([['protocol', 'probe.protocol'], ['src', 'probe.src'], ['sport', 'probe.sport'], ['dst', 'probe.dst'], ['dport', 'probe.dport']] as const).map(([field, label]) => (
          <div className="field" key={field}>
            <label htmlFor={`probe-${field}`}>{t(label)}</label>
            <input id={`probe-${field}`} value={form[field]} spellCheck={false} inputMode={field === 'src' || field === 'dst' ? 'decimal' : 'numeric'}
              onChange={(e) => setForm({ ...form, [field]: e.target.value })} />
          </div>
        ))}
        <button type="submit" className="button button--secondary button--small" disabled={review.busy !== null}>{t('probe.run')}</button>
      </form>
      {invalid && <p className="field-error" role="alert">{t('probe.invalid')}</p>}
      {review.busy === 'probe' && <BusyPanel label={t('probe.busy')} onCancel={review.cancel} />}
      {result && (
        <dl className="probe-result" aria-live="polite">
          <div><dt>{t('changes.example')}</dt><dd><code>{packetText(result.packet)}</code></dd></div>
          {row(t('changes.before'), result.before, source.before, first.before)}
          {row(t('changes.after'), result.after, source.after, first.after)}
          {result.candidate && review.repair?.receipt.candidate && row(t('probe.candidate'), result.candidate, review.repair.receipt.candidate, 1)}
        </dl>
      )}
    </TechnicalDetails>
    </div>
  );
}
