import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { PacketRoute } from '../src/components/PacketRoute';
import { LocaleProvider, translate } from '../src/i18n';
import { witness } from './fixtures';

afterEach(cleanup);

describe('concrete witness packet ports', () => {
  for (const locale of ['vi', 'en'] as const) {
    for (const protocol of [6, 17]) {
      it(`${locale}: preserves zero and nonzero ports for protocol ${protocol}`, () => {
        localStorage.setItem('plenilune.locale', locale);
        const { rerender } = render(<LocaleProvider><PacketRoute packet={{ ...witness.packet, protocol, src_port: 0, dst_port: 0 }} /></LocaleProvider>);
        const route = screen.getByRole('group', { name: translate(locale, 'packet.route') });
        expect(Array.from(route.querySelectorAll('.packet-endpoint code'), node => node.textContent)).toEqual([':0', ':0']);
        rerender(<LocaleProvider><PacketRoute packet={{ ...witness.packet, protocol, src_port: 49152, dst_port: 443 }} /></LocaleProvider>);
        expect(Array.from(route.querySelectorAll('.packet-endpoint code'), node => node.textContent)).toEqual([':49152', ':443']);
      });
    }
    it(`${locale}: labels ports as inapplicable for ICMP`, () => {
      localStorage.setItem('plenilune.locale', locale);
      render(<LocaleProvider><PacketRoute packet={{ ...witness.packet, protocol: 1, src_port: 0, dst_port: 0 }} /></LocaleProvider>);
      expect(screen.getAllByText(translate(locale, 'packet.notTcp'))).toHaveLength(2);
      expect(screen.queryByText(':0')).toBeNull();
    });
  }
});
