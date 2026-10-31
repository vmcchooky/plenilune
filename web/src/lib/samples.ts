import type { CompareOptions, Obligation, Sample } from './types';

const before = `ip access-list extended MGMT
 10 remark synthetic documentation addresses only
 20 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22
 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443
 40 deny ip any any
!
`;

const after = `ip access-list extended MGMT
 10 remark synthetic documentation addresses only
 20 deny tcp host 192.0.2.10 any eq 22 log
 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22
 40 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443
 50 deny ip any any
!
`;

const intent = 'Every management host must retain SSH access to the primary jump host. SSH to the backup host must remain denied, and existing HTTPS access must not change.';

const conditions: Obligation[] = [
  {
    id: 'OB-1',
    kind: 'must_permit',
    match: {
      protocol: 'tcp',
      src_cidr: '192.0.2.0/24',
      dst_cidr: '198.51.100.10/32',
      exclude_src: [],
      exclude_dst: [],
      src_ports: [],
      dst_ports: [{ lo: 22, hi: 22 }],
    },
  },
  {
    id: 'OB-2',
    kind: 'must_deny',
    match: {
      protocol: 'tcp',
      src_cidr: '192.0.2.0/24',
      dst_cidr: '198.51.100.20/32',
      exclude_src: [],
      exclude_dst: [],
      src_ports: [],
      dst_ports: [{ lo: 22, hi: 22 }],
    },
  },
  {
    id: 'OB-3',
    kind: 'must_remain',
    match: {
      protocol: 'tcp',
      src_cidr: '192.0.2.0/24',
      dst_cidr: '198.51.100.10/32',
      exclude_src: [],
      exclude_dst: [],
      src_ports: [],
      dst_ports: [{ lo: 443, hi: 443 }],
    },
  },
];

const limitedBudget: CompareOptions = {
  limits: {
    max_operations: 50,
    max_boxes: 500,
    timeout_ns: 2_000_000_000,
  },
  max_witnesses: 8,
};


// Full synthetic configurations: the review starts from the whole file and
// Plenilune isolates supported objects with their source lines.
const fullConfigs = {
  regression: { before: "! Synthetic configuration - documentation addresses only\nhostname EDGE-R1\n!\ninterface GigabitEthernet0/1\n description user-lan\n ip address 192.0.2.1 255.255.255.0\n ip access-group MGMT in\n!\nip access-list extended MGMT\n 10 remark management access\n 20 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443\n 40 permit udp host 192.0.2.53 any eq 53\n 50 deny ip any any log\n!\nip prefix-list CUSTOMER seq 5 permit 203.0.113.0/24\nip prefix-list CUSTOMER seq 10 deny 0.0.0.0/0 le 32\n!\nrouter bgp 64500\n neighbor 198.51.100.2 remote-as 64501\n neighbor 198.51.100.2 prefix-list CUSTOMER in\n!\nend\n", after: "! Synthetic configuration - documentation addresses only\nhostname EDGE-R1\n!\ninterface GigabitEthernet0/1\n description user-lan\n ip address 192.0.2.1 255.255.255.0\n ip access-group MGMT in\n!\nip access-list extended MGMT\n 10 remark management access\n 15 deny tcp host 192.0.2.10 any eq 22\n 20 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443\n 40 permit udp host 192.0.2.53 any eq 53\n 45 permit tcp any any eq 3389\n 50 deny ip any any log\n!\nip prefix-list CUSTOMER seq 5 permit 203.0.113.0/24\nip prefix-list CUSTOMER seq 10 deny 0.0.0.0/0 le 32\n!\nrouter bgp 64500\n neighbor 198.51.100.2 remote-as 64501\n neighbor 198.51.100.2 prefix-list CUSTOMER in\n!\nend\n" },
  routing: { before: "! Synthetic configuration - documentation addresses only\nhostname EDGE-R1\n!\ninterface GigabitEthernet0/1\n description user-lan\n ip address 192.0.2.1 255.255.255.0\n ip access-group MGMT in\n!\nip access-list extended MGMT\n 10 remark management access\n 20 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443\n 40 permit udp host 192.0.2.53 any eq 53\n 50 deny ip any any log\n!\nip prefix-list CUSTOMER seq 5 permit 203.0.113.0/24\nip prefix-list CUSTOMER seq 10 deny 0.0.0.0/0 le 32\n!\nrouter bgp 64500\n neighbor 198.51.100.2 remote-as 64501\n neighbor 198.51.100.2 prefix-list CUSTOMER in\n!\nend\n", after: "! Synthetic configuration - documentation addresses only\nhostname EDGE-R1\n!\ninterface GigabitEthernet0/1\n description user-lan\n ip address 192.0.2.1 255.255.255.0\n ip access-group MGMT in\n!\nip access-list extended MGMT\n 10 remark management access\n 20 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443\n 40 permit udp host 192.0.2.53 any eq 53\n 50 deny ip any any log\n!\nip prefix-list CUSTOMER seq 3 deny 203.0.113.128/25 le 32\nip prefix-list CUSTOMER seq 5 permit 203.0.113.0/24 le 26\nip prefix-list CUSTOMER seq 10 deny 0.0.0.0/0 le 32\n!\nrouter bgp 64500\n neighbor 198.51.100.2 remote-as 64501\n neighbor 198.51.100.2 prefix-list CUSTOMER in\n!\nend\n" },
  refactor: { before: "! Synthetic configuration - documentation addresses only\nhostname EDGE-R1\n!\ninterface GigabitEthernet0/1\n description user-lan\n ip address 192.0.2.1 255.255.255.0\n ip access-group MGMT in\n!\nip access-list extended MGMT\n 10 remark management access\n 20 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 22\n 30 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443\n 40 permit udp host 192.0.2.53 any eq 53\n 50 deny ip any any log\n!\nip prefix-list CUSTOMER seq 5 permit 203.0.113.0/24\nip prefix-list CUSTOMER seq 10 deny 0.0.0.0/0 le 32\n!\nrouter bgp 64500\n neighbor 198.51.100.2 remote-as 64501\n neighbor 198.51.100.2 prefix-list CUSTOMER in\n!\nend\n", after: "! Synthetic configuration - documentation addresses only\nhostname EDGE-R1\n!\ninterface GigabitEthernet0/1\n description user-lan\n ip address 192.0.2.1 255.255.255.0\n ip access-group MGMT in\n!\nip access-list extended MGMT\n 10 remark management access, split per half subnet\n 20 permit tcp 192.0.2.0 0.0.0.127 host 198.51.100.10 eq 22\n 25 permit tcp 192.0.2.128 0.0.0.127 host 198.51.100.10 eq 22\n 30 permit udp host 192.0.2.53 any eq 53\n 40 permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443\n 50 remark everything else is dropped\n 60 deny ip any any\n!\nip prefix-list CUSTOMER seq 100 permit 203.0.113.0/24\nip prefix-list CUSTOMER seq 200 deny 0.0.0.0/0 le 32\n!\nrouter bgp 64500\n neighbor 198.51.100.2 remote-as 64501\n neighbor 198.51.100.2 prefix-list CUSTOMER in\n!\nend\n" },
};

export const samples: Sample[] = [
  {
    id: 'ssh-drift',
    nameKey: 'source.sample.ssh',
    before,
    after,
    intent,
    conditions,
    scopeMode: 'none',
    allowedChanges: [],
  },
  {
    id: 'budget-exhausted',
    nameKey: 'source.sample.budget',
    before,
    after,
    intent,
    conditions,
    scopeMode: 'none',
    allowedChanges: [],
    options: limitedBudget,
  },
  {
    id: 'full-regression',
    nameKey: 'source.sample.fullRegression',
    before: fullConfigs.regression.before,
    after: fullConfigs.regression.after,
    intent,
    conditions,
    scopeMode: 'none',
    allowedChanges: [],
  },
  {
    id: 'full-routing',
    nameKey: 'source.sample.fullRouting',
    before: fullConfigs.routing.before,
    after: fullConfigs.routing.after,
    intent: 'Accept the customer /24 and its /25-/26 subnets except the upper half; keep everything else filtered.',
    conditions,
    scopeMode: 'none',
    allowedChanges: [],
  },
  {
    id: 'full-refactor',
    nameKey: 'source.sample.fullRefactor',
    before: fullConfigs.refactor.before,
    after: fullConfigs.refactor.after,
    intent: 'Pure cleanup: renumbering and splitting rules must not change any decision.',
    conditions,
    scopeMode: 'none',
    allowedChanges: [],
  },
  {
    id: 'unsupported-syntax',
    nameKey: 'source.sample.unsupported',
    before,
    after: after.replace('permit tcp 192.0.2.0 0.0.0.255 host 198.51.100.10 eq 443', 'permit tcp 192.0.2.0 0.0.0.255 object-group WEB eq 443'),
    intent,
    conditions,
    scopeMode: 'none',
    allowedChanges: [],
  },
];

export const blankSample: Sample = {
  id: 'blank',
  nameKey: 'source.sample.ssh',
  before: '',
  after: '',
  intent: '',
  conditions: [],
  scopeMode: 'undeclared',
  allowedChanges: [],
};

export function findSample(id: string): Sample | null {
  return samples.find((sample) => sample.id === id) ?? null;
}

export function isPristineSample(sample: Sample | null, draft: { before: string; after: string; intent: string }): boolean {
  return sample !== null && sample.before === draft.before && sample.after === draft.after && sample.intent === draft.intent;
}
