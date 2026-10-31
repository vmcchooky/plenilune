import type { Locale } from '../i18n';

export function shortHash(hash: string): string {
  if (hash.length <= 18) return hash;
  return `${hash.slice(0, 10)}…${hash.slice(-6)}`;
}

export function formatTimestamp(locale: Locale, date = new Date()): string {
  return new Intl.DateTimeFormat(locale === 'vi' ? 'vi-VN' : 'en-GB', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

export function fileStamp(date = new Date()): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function prettyJson(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

export function packetPort(protocol: number, port: number): string | null {
  return protocol === 6 || protocol === 17 ? String(port) : null;
}

export function protocolName(protocol: number, tcpLabel = 'TCP', udpLabel = 'UDP', icmpLabel = 'ICMP'): string {
  switch (protocol) {
    case 6: return tcpLabel;
    case 17: return udpLabel;
    case 1: return icmpLabel;
    default: return String(protocol);
  }
}

export function joinList(values: string[]): string {
  return values.join(', ');
}
