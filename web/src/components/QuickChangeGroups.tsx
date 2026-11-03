import { useI18n } from '../i18n';
import type { ChangeClass } from '../lib/types';
import { Icon } from './Icon';

export function QuickChangeGroups({ groups, selected, onSelect }: {
  groups: ChangeClass[]; selected: number | null; onSelect: (index: number) => void;
}) {
  const { t } = useI18n();
  return <section className="record-change-groups" aria-label={t('quick.groupList')}>
    <p>{t('quick.pickGroup')}</p>
    <ol>{groups.map((group, index) => {
      const region = group.examples[0];
      return <li key={index}><button type="button" aria-pressed={selected === index} onClick={() => onSelect(index)}>
        <span>{index + 1}</span>
        <span><strong>{t(`changes.effect.${group.effect}`)}</strong>
          {region && <code>{region.protocol.toUpperCase()} {region.src} → {region.dst} · {region.src_ports} → {region.dst_ports}</code>}
        </span>
        <Icon name="chevron" size={16} />
      </button></li>;
    })}</ol>
  </section>;
}
