import { useState } from 'react';
import type { CSSProperties } from 'react';
import { ChangeTitle, ConditionsAction, ScopeNote, Transition } from './Shared';
import type { CompositionProps } from './Shared';
import { protocolText } from './model';

export function InlineComposition({ model, copy, onConditions }: CompositionProps) {
  const [side, setSide] = useState<'before' | 'after'>('after');
  const lines = model[side].replace(/\r\n/g, '\n').replace(/\n$/, '').split('\n');
  const decision = model.witness[side];
  const annotationStyle = { '--decision-offset': Math.max(0, (decision.line ?? 1) - 1) } as CSSProperties;
  return <article className="study-inline" aria-labelledby="inline-title">
    <header className="study-inline__head">
      <div><p className="study-object">{model.name} <span>{copy.source}</span></p><h1 id="inline-title"><ChangeTitle model={model} copy={copy} /></h1></div>
      <div className="study-side-switch" role="group" aria-label={copy.source}>
        {(['before', 'after'] as const).map(value => <button type="button" key={value} aria-pressed={side === value} onClick={() => setSide(value)}>{copy[value]}</button>)}
      </div>
    </header>
    <div className="study-inline__workspace">
      <section className="study-code" aria-label={`${copy.source} · ${copy[side]}`}>
        <div className="study-code__head"><strong>{copy[side]} · {model.name}</strong><span>{lines.length} {copy.lines}</span></div>
        <div className="study-code__scroll" tabIndex={0} role="region" aria-label={`${copy.source} · ${copy[side]}`}><ol>{lines.map((line, i) => <li key={i} className={decision.line === i + 1 ? 'is-deciding' : ''}>
          <span>{i + model.first[side]}</span><code>{line || ' '}</code>
          {decision.line === i + 1 && <small>{copy.matched}</small>}
        </li>)}</ol></div>
      </section>
      <aside className="study-inline__effect" style={annotationStyle}>
        <span className="study-caption">{side === 'after' ? copy.effect : copy.compareEffect}</span>
        <h2>{protocolText(model.witness)}</h2>
        <p className="study-inline__packet">{model.witness.packet.src_ip}<br />→ {model.witness.packet.dst_ip}:{model.witness.packet.dst_port}</p>
        <p className="study-caption">{copy.example} · {copy.sourcePort} {model.witness.packet.src_port}</p>
        <Transition witness={model.witness} copy={copy} />
        <ConditionsAction copy={copy} onConditions={onConditions} />
      </aside>
    </div>
    <ScopeNote model={model} copy={copy} />
  </article>;
}
