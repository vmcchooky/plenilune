import { useI18n } from '../i18n';
import { packetPort } from '../lib/format';
import type { Packet } from '../lib/types';
import { Icon } from './Icon';

export function PacketRoute({ packet }: { packet: Packet }) {
  const { t } = useI18n();
  const srcPort = packetPort(packet.protocol, packet.src_port);
  const dstPort = packetPort(packet.protocol, packet.dst_port);
  const protocol = packet.protocol === 6 ? 'TCP'
    : packet.protocol === 17 ? 'UDP'
      : packet.protocol === 1 ? 'ICMP'
        : t('format.protocolNumber', { n: packet.protocol });
  return (
    <div className="packet-route" role="group" aria-label={t('packet.route')}>
      <div className="packet-endpoint">
        <span>{t('packet.src')}</span>
        <strong>{packet.src_ip}</strong>
        <code>{srcPort ? `:${srcPort}` : t('packet.notTcp')}</code>
      </div>
      <div className="packet-route__middle">
        <span className="packet-protocol" title={t('packet.rawProtocol', { n: packet.protocol })}>
          {protocol}
        </span>
        <span className="packet-route__line" aria-hidden="true" />
        <Icon name="arrow" size={18} />
      </div>
      <div className="packet-endpoint">
        <span>{t('packet.dst')}</span>
        <strong>{packet.dst_ip}</strong>
        <code>{dstPort ? `:${dstPort}` : t('packet.notTcp')}</code>
      </div>
    </div>
  );
}
