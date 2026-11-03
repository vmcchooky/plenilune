import { useState } from 'react';
import { ConditionsAction, PacketRoute, RuleProof, ScopeNote, Transition } from './Shared';
import type { CompositionProps } from './Shared';
import { protocolText } from './model';
import { Icon } from '../../components/Icon';

export function WorkbenchComposition({ model, copy, onConditions }: CompositionProps) {
  const [index, setIndex] = useState(0);
  const groups = model.changes?.classes ?? [];
  const selected = groups[index] ?? groups[0];
  const witness = selected?.witness ?? model.witness;
  const region = selected?.examples[0];
  return <article className="study-workbench" aria-labelledby="workbench-title">
    <header className="study-workbench__head"><div><p className="study-caption">{copy.check}</p><h1 id="workbench-title">{model.name}</h1></div><span>{model.changes?.total_classes ?? '—'} {copy.groups}</span></header>
    <div className="study-workbench__body">
      <nav className="study-queue" aria-label={copy.choose}>
        <p>{copy.choose}</p>
        {groups.length ? groups.map((group, i) => <button type="button" key={i} aria-pressed={i === index} onClick={() => setIndex(i)}>
          <span>{String(i + 1).padStart(2, '0')}</span>
          <div><strong>{group.effect === 'newly_denied' ? copy.newDeny : copy.newPermit}</strong><small>{protocolText(group.witness)}</small></div>
          <Icon name="chevron" size={16} />
        </button>) : <p>{copy.noClasses}</p>}
      </nav>
      <section className="study-group" aria-label={copy.evidence}>
        <div className="study-group__heading"><h2>{model.incomplete ? copy.incomplete : selected?.effect === 'newly_permitted' ? copy.newPermit : copy.newDeny}</h2><ConditionsAction copy={copy} onConditions={onConditions} /></div>
        {region && <div className="study-region">
          <span>{selected?.pieces === 1 && !selected.examples_truncated ? copy.region : copy.regionExample}</span>
          <p><strong>{region.protocol.toUpperCase()}</strong> {region.src} → {region.dst}</p>
          <small>{copy.sourcePort}: {region.src_ports === 'any' ? copy.any : region.src_ports} · {copy.destinationPort}: {region.dst_ports === 'any' ? copy.any : region.dst_ports}</small>
        </div>}
        <Transition witness={witness} copy={copy} />
        <RuleProof model={model} copy={copy} witness={witness} />
        <details className="study-more"><summary>{copy.example}</summary><PacketRoute witness={witness} copy={copy} /></details>
      </section>
    </div>
    <ScopeNote model={model} copy={copy} />
  </article>;
}
