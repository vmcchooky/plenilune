import type { Witness } from '../../lib/types';
import { Icon } from '../../components/Icon';
import type { CompositionCopy } from './copy';
import { decidingNumber, decidingText } from './model';
import type { CompositionModel } from './model';

export type CompositionProps = { model: CompositionModel; copy: CompositionCopy; onConditions: () => void };
export function ChangeTitle({ model, copy }: Pick<CompositionProps, 'model' | 'copy'>) {
  return <>{model.incomplete ? copy.incomplete : model.witness.after.action === 'deny' ? copy.title : copy.permitted}</>;
}
export function Transition({ witness, copy }: { witness: Witness; copy: CompositionCopy }) {
  return <div className="study-transition">
    <span>{copy.before} <strong className={`study-status study-status--${witness.before.action}`}>{copy[witness.before.action]}</strong></span>
    <Icon name="arrow" size={16} />
    <span>{copy.after} <strong className={`study-status study-status--${witness.after.action}`}>{copy[witness.after.action]}</strong></span>
  </div>;
}
export function PacketRoute({ witness, copy }: { witness: Witness; copy: CompositionCopy }) {
  const packet = witness.packet;
  const ports = packet.protocol === 6 || packet.protocol === 17;
  return <div className="study-packet">
    <div><span>{copy.from}</span><strong>{packet.src_ip}</strong>{ports && <small>{copy.sourcePort} {packet.src_port}</small>}</div>
    <Icon name="arrow" size={22} />
    <div><span>{copy.to}</span><strong>{packet.dst_ip}{ports && <em>:{packet.dst_port}</em>}</strong></div>
  </div>;
}
export function RuleProof({ model, copy, witness = model.witness }: Pick<CompositionProps, 'model' | 'copy'> & { witness?: Witness }) {
  return <div className="study-proof">{(['before', 'after'] as const).map(side => (
    <div className="study-proof__side" key={side}>
      <div><span>{copy[side]}</span><strong>{copy[witness[side].action]}</strong></div>
      <p>{witness[side].implicit ? copy.implicit : `${copy.rule} ${decidingNumber(model, side, witness[side]) ?? '—'}`}</p>
      {decidingText(model, side, witness[side]) && <code>{decidingText(model, side, witness[side])}</code>}
    </div>
  ))}</div>;
}
export function ConditionsAction({ copy, onConditions }: Pick<CompositionProps, 'copy' | 'onConditions'>) {
  return <button type="button" className="button button--primary study-conditions" onClick={onConditions}>
    {copy.conditions}<Icon name="arrow" size={16} />
  </button>;
}
export function ScopeNote({ model, copy, className = 'study-scope' }: Pick<CompositionProps, 'model' | 'copy'> & { className?: string }) {
  return <div className={className}>
    {(model.incomplete || model.truncated || model.coverageGap) && <p role="status" className="banner banner--warn">
      {model.incomplete && `${copy.incomplete}. `}{model.truncated && `${copy.truncated} `}{model.coverageGap && copy.coverage}
    </p>}
    <p>{copy.limit} {model.policyAccepted ? model.conditionsUnresolved ? copy.rejected : copy.accepted : copy.intent}</p>
  </div>;
}
