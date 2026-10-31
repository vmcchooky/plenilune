import { useI18n } from '../i18n';
import { parsePorts, protocolLabel } from '../lib/conditions';
import type { RegionDraft } from '../lib/conditions';
import { TechnicalDetails } from './Ui';

const baseProtocols = ['ip', 'tcp', 'udp', 'icmp'];

export function RegionFields({
  idPrefix,
  value,
  onChange,
  disabled = false,
}: {
  idPrefix: string;
  value: RegionDraft;
  onChange: (next: RegionDraft) => void;
  disabled?: boolean;
}) {
  const { t } = useI18n();
  const portsApply = value.protocol === 'tcp' || value.protocol === 'udp';
  const srcPortError = portsApply && parsePorts(value.src_ports_text) === null;
  const dstPortError = portsApply && parsePorts(value.dst_ports_text) === null;
  const protocols = baseProtocols.includes(value.protocol) ? baseProtocols : [...baseProtocols, value.protocol];

  function set<K extends keyof RegionDraft>(field: K, next: RegionDraft[K]) {
    onChange({ ...value, [field]: next });
  }

  return (
    <div className="region-fields">
      <div className="field">
        <label htmlFor={`${idPrefix}-protocol`}>{t('cond.protocol')}</label>
        <select
          id={`${idPrefix}-protocol`}
          value={value.protocol}
          disabled={disabled}
          onChange={(event) => set('protocol', event.target.value)}
        >
          {protocols.map((protocol) => (
            <option key={protocol} value={protocol}>{protocolLabel(t, protocol)}</option>
          ))}
        </select>
      </div>
      <div className="field">
        <label htmlFor={`${idPrefix}-src`}>{t('cond.src')}</label>
        <input
          id={`${idPrefix}-src`}
          value={value.src_cidr}
          disabled={disabled}
          spellCheck={false}
          placeholder={t('cond.cidrPlaceholder')}
          onChange={(event) => set('src_cidr', event.target.value)}
        />
      </div>
      <div className="field">
        <label htmlFor={`${idPrefix}-dst`}>{t('cond.dst')}</label>
        <input
          id={`${idPrefix}-dst`}
          value={value.dst_cidr}
          disabled={disabled}
          spellCheck={false}
          placeholder={t('cond.cidrPlaceholder')}
          onChange={(event) => set('dst_cidr', event.target.value)}
        />
      </div>
      <div className="field">
        <label htmlFor={`${idPrefix}-dst-ports`}>{t('cond.dstPorts')}</label>
        <input
          id={`${idPrefix}-dst-ports`}
          value={value.dst_ports_text}
          disabled={disabled || !portsApply}
          spellCheck={false}
          placeholder={portsApply ? t('cond.portsPlaceholder') : t('cond.portsOnlyTcp')}
          aria-invalid={dstPortError || undefined}
          aria-describedby={dstPortError ? `${idPrefix}-dst-ports-error` : undefined}
          onChange={(event) => set('dst_ports_text', event.target.value)}
        />
        {dstPortError && <p className="field-error" id={`${idPrefix}-dst-ports-error`} role="alert">{t('cond.portError')}</p>}
      </div>
      <TechnicalDetails title={t('cond.excludes')} className="region-fields__advanced">
        <div className="region-fields__advanced-grid">
          <div className="field">
            <label htmlFor={`${idPrefix}-src-ports`}>{t('cond.srcPorts')}</label>
            <input
              id={`${idPrefix}-src-ports`}
              value={value.src_ports_text}
              disabled={disabled || !portsApply}
              spellCheck={false}
              placeholder={portsApply ? t('cond.portsPlaceholder') : t('cond.portsOnlyTcp')}
              aria-invalid={srcPortError || undefined}
              aria-describedby={srcPortError ? `${idPrefix}-src-ports-error` : undefined}
              onChange={(event) => set('src_ports_text', event.target.value)}
            />
            {srcPortError && <p className="field-error" id={`${idPrefix}-src-ports-error`}>{t('cond.portError')}</p>}
          </div>
          <div className="field">
            <label htmlFor={`${idPrefix}-exclude-src`}>{t('cond.excludeSrc')}</label>
            <input
              id={`${idPrefix}-exclude-src`}
              value={value.exclude_src_text}
              disabled={disabled}
              spellCheck={false}
              placeholder="192.0.2.99/32, 198.51.100.0/24"
              onChange={(event) => set('exclude_src_text', event.target.value)}
            />
          </div>
          <div className="field">
            <label htmlFor={`${idPrefix}-exclude-dst`}>{t('cond.excludeDst')}</label>
            <input
              id={`${idPrefix}-exclude-dst`}
              value={value.exclude_dst_text}
              disabled={disabled}
              spellCheck={false}
              placeholder="198.51.100.99/32"
              onChange={(event) => set('exclude_dst_text', event.target.value)}
            />
          </div>
        </div>
      </TechnicalDetails>
    </div>
  );
}
