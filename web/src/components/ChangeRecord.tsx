import { useState } from 'react';
import type { ReactNode } from 'react';
import type { Verdict, Witness } from '../lib/types';
import { Icon } from './Icon';
import type { IconName } from './Icon';
import '../styles/change-record.css';

export interface ChangeRecordCopy {
  aclObject: string; before: string; after: string; permit: string; deny: string;
  example: string; from: string; to: string; implicit: string; decidingReason: string;
  rule: string; proof: string; conditions: string; groups: string;
}
interface ChangeRecordProps {
  name: string; title: string; copy: ChangeRecordCopy; witness?: Witness;
  cause?: { implicit: boolean; line?: number; text?: string | null };
  groupCount?: number; verdict?: Verdict; verdictLabel?: string; className?: string;
  headingId?: string; headingLevel?: 1 | 2; proofId?: string;
  proof?: ReactNode; scope?: ReactNode; notices?: ReactNode; details?: ReactNode;
  leadingContent?: ReactNode; onConditions?: () => void;
  secondaryAction?: { label: string; run: () => void };
  // A stronger action than the conditions CTA (e.g. the budget retry on an
  // inconclusive verdict): renders primary, conditions drops to secondary.
  primaryAction?: { label: string; run: () => void; icon?: IconName };
}

// Presentation only: callers supply the engine result and callbacks. Selection,
// policy acceptance and verdict computation stay with their existing owners.
export function ChangeRecord({ name, title, copy, witness, cause, groupCount = 0,
  verdict, verdictLabel, className = '', headingId = 'change-record-title', headingLevel = 2,
  proofId = 'change-record-proof', proof, scope, notices, details, leadingContent,
  onConditions, secondaryAction, primaryAction }: ChangeRecordProps) {
  const [proofOpen, setProofOpen] = useState(false);
  const Heading = headingLevel === 1 ? 'h1' : 'h2';
  const packet = witness?.packet;
  const ports = packet?.protocol === 6 || packet?.protocol === 17;
  const protocol = packet ? packet.protocol === 6 ? 'TCP' : packet.protocol === 17 ? 'UDP'
    : packet.protocol === 1 ? 'ICMP' : String(packet.protocol) : null;
  const tone = verdict === 'DIFFERENT' ? 'bad' : verdict === 'INCONCLUSIVE' ? 'warn' : 'neutral';
  return <section className={`record-refined ${className}`} aria-labelledby={headingId}>
    <header className="record-refined__context quick-outcome__identity">
      <span>{copy.aclObject} <strong>{name}</strong></span>
      {packet && <span className="record-refined__protocol">{protocol}{ports ? `/${packet.dst_port}` : ''}</span>}
      {verdict && <code className={`badge badge--${tone}`}>{verdictLabel ?? verdict}</code>}
      {groupCount > 1 && <span className="record-refined__meta">{groupCount} {copy.groups}</span>}
    </header>
    {notices}
    <div className="record-refined__body">
      <div className="record-refined__heading">
        <Heading id={headingId} className="record-refined__title">{title}</Heading>
        {witness && <div className="record-refined__transition">
          <span>{copy.before} <strong>{copy[witness.before.action]}</strong></span>
          <Icon name="arrow" size={16} />
          <span>{copy.after} <strong className={`record-refined__status record-refined__status--${witness.after.action}`}>{copy[witness.after.action]}</strong></span>
        </div>}
      </div>
      {leadingContent}
      {packet && <section className="record-refined__packet quick-outcome__example" aria-label={copy.example}>
        <p>{copy.example}</p>
        <div className="record-flow">
          <div><span>{copy.from}</span><strong>{packet.src_ip}{ports && <em>:{packet.src_port}</em>}</strong></div>
          <Icon name="arrow" size={22} />
          <div><span>{copy.to}</span><strong>{packet.dst_ip}{ports && <em>:{packet.dst_port}</em>}</strong></div>
        </div>
      </section>}
      {cause && <div className="record-refined__cause quick-outcome__rule">
        <div><span>{copy.decidingReason}</span><span>{cause.implicit ? copy.implicit : `${copy.after} · ${copy.rule} ${cause.line ?? '—'}`}</span></div>
        {cause.text && <code>{cause.text}</code>}
      </div>}
    </div>
    <footer className="record-refined__actions">
      {proof && <button type="button" className="button button--quiet record-refined__proof-toggle"
        aria-expanded={proofOpen} aria-controls={proofId} onClick={() => setProofOpen(!proofOpen)}>
        <Icon name="chevron" size={16} />{copy.proof}
      </button>}
      {secondaryAction && <button type="button" className="button button--quiet" onClick={secondaryAction.run}>{secondaryAction.label}</button>}
      {onConditions && <button type="button" className={`button ${primaryAction ? 'button--secondary' : 'button--primary'} study-conditions`} onClick={onConditions}>
        {copy.conditions}<Icon name="arrow" size={16} />
      </button>}
      {primaryAction && <button type="button" className="button button--primary" onClick={primaryAction.run}>
        {primaryAction.icon && <Icon name={primaryAction.icon} size={16} />}{primaryAction.label}
      </button>}
    </footer>
    {proof && <div className="record-refined__proof" id={proofId} hidden={!proofOpen}>{proof}</div>}
    {scope}
    {details}
  </section>;
}
