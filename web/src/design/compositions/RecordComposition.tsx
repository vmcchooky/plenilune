import { ChangeRecord } from '../../components/ChangeRecord';
import { RuleProof, ScopeNote } from './Shared';
import type { CompositionProps } from './Shared';
import { decidingNumber, decidingText } from './model';

export function RecordComposition({ model, copy, onConditions }: CompositionProps) {
  const rule = decidingText(model, 'after', model.witness.after);
  const line = decidingNumber(model, 'after', model.witness.after);
  const title = model.incomplete ? copy.incomplete : model.witness.after.action === 'deny' ? copy.title : copy.permitted;
  return <ChangeRecord name={model.name} title={title} copy={copy} witness={model.witness}
    headingId="record-title" headingLevel={1} proofId="record-rule-proof"
    groupCount={model.changes?.total_classes} onConditions={onConditions}
    cause={{ implicit: model.witness.after.implicit, line, text: rule }}
    proof={<RuleProof model={model} copy={copy} />}
    scope={<ScopeNote model={model} copy={copy} className="record-refined__scope" />} />;
}
