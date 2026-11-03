"""Synthetic ACL differential oracle. No Plenilune IR/algebra is imported.

Z3 uses nested first-match ITEs over full-width bitvectors. The generator's
tuples independently produce IOS text and the SMT formula. Unknown, CLI errors,
INCONCLUSIVE and witness mismatches fail the gate; failures retain repro input.
"""
import argparse
import ipaddress
import json
from pathlib import Path
import random
import subprocess
import tempfile

import z3


def ip(n):
    return str(ipaddress.IPv4Address(n))


def address(a):
    value, wildcard = a
    return 'any' if wildcard == 0xffffffff else f'{ip(value)} {ip(wildcard)}'


def port_text(p):
    return '' if p is None else ' ' + ' '.join(map(str, p))


def config(rules):
    return 'ip access-list extended ORACLE\n' + ''.join(
        f' {action} {"ip" if proto is None else proto} {address(src)}{port_text(sp)} '
        f'{address(dst)}{port_text(dp)}\n'
        for action, proto, src, dst, sp, dp in rules)


def port_match(x, p):
    if p is None:
        return z3.BoolVal(True)
    op, *v = p
    return {'eq': lambda: x == v[0], 'neq': lambda: x != v[0],
            'lt': lambda: z3.ULT(x, v[0]), 'gt': lambda: z3.UGT(x, v[0]),
            'range': lambda: z3.And(z3.UGE(x, v[0]), z3.ULE(x, v[1]))}[op]()


def decision(rules, packet):
    proto, src, dst, sp, dp = packet
    result = z3.BoolVal(False)  # implicit deny
    for action, protocol, source, dest, sport, dport in reversed(rules):
        match = z3.And(
            z3.BoolVal(True) if protocol is None else proto == protocol,
            (src & (~source[1] & 0xffffffff)) == (source[0] & (~source[1] & 0xffffffff)),
            (dst & (~dest[1] & 0xffffffff)) == (dest[0] & (~dest[1] & 0xffffffff)),
            port_match(sp, sport), port_match(dp, dport))
        result = z3.If(match, z3.BoolVal(action == 'permit'), result)
    return result


def concrete(packet):
    return (z3.BitVecVal(packet['protocol'], 8),
            z3.BitVecVal(int(ipaddress.IPv4Address(packet['src_ip'])), 32),
            z3.BitVecVal(int(ipaddress.IPv4Address(packet['dst_ip'])), 32),
            z3.BitVecVal(packet['src_port'], 16), z3.BitVecVal(packet['dst_port'], 16))


def action(rules, packet):
    return 'permit' if z3.is_true(z3.simplify(decision(rules, concrete(packet)))) else 'deny'


def run(binary, args):
    p = subprocess.run([str(binary), *args], capture_output=True, text=True, timeout=30)
    if p.returncode not in (0, 1):
        raise RuntimeError(f'CLI exit {p.returncode}: {p.stderr}')
    return json.loads(p.stdout)


def check(binary, before, after, directory):
    b, a = directory / 'before.acl', directory / 'after.acl'
    b.write_text(config(before), encoding='utf-8')
    a.write_text(config(after), encoding='utf-8')
    packet = tuple(z3.BitVec(n, w) for n, w in zip(('proto', 'src', 'dst', 'sp', 'dp'), (8, 32, 32, 16, 16)))
    solver = z3.Solver()
    solver.set(timeout=10000)
    solver.add(z3.Or(packet[0] == 6, packet[0] == 17, z3.And(packet[3] == 0, packet[4] == 0)))
    solver.add(decision(before, packet) != decision(after, packet))
    result = solver.check()
    if result == z3.unknown:
        raise AssertionError('SMT unknown: ' + solver.reason_unknown())
    expected = 'DIFFERENT' if result == z3.sat else 'EQUIVALENT'
    receipt = run(binary, ['diff', '--before', str(b), '--after', str(a)])
    report = receipt['report']
    assert report['verdict'] == expected, (expected, report)
    assert report['search_complete'], report
    for witness in report['witnesses']:
        p = witness['packet']
        assert action(before, p) == witness['before']['action']
        assert action(after, p) == witness['after']['action']
        assert action(before, p) != action(after, p)
    if expected == 'DIFFERENT':
        assert report['witnesses'], 'Missing engine witness'
        model = solver.model()
        values = [model.eval(x, model_completion=True).as_long() for x in packet]
        p = dict(zip(('protocol', 'src_ip', 'dst_ip', 'src_port', 'dst_port'), values))
        p['src_ip'], p['dst_ip'] = ip(values[1]), ip(values[2])
        path = directory / 'packet.json'
        path.write_text(json.dumps(p), encoding='utf-8')
        for source, rules in ((b, before), (a, after)):
            trace = run(binary, ['trace', '--config', str(source), '--packet', str(path)])
            assert trace['decision']['action'] == action(rules, p), trace
    return expected


def corpus(seed, count):
    rng = random.Random(seed)
    any_addr = (0, 0xffffffff)
    permit = ('permit', None, any_addr, any_addr, None, None)
    deny = ('deny', None, any_addr, any_addr, None, None)
    yield [permit], [deny]
    yield [deny], [deny, permit]  # first-match shadowing
    yield [deny], [('deny', 6, any_addr, any_addr, None, None)]  # implicit deny
    yield [permit], [permit, deny]
    addresses = [any_addr, (0xc0000201, 0), (0xc0000200, 255),
                 (0xc6336400, 0x00000005), (0xcb007100, 0x000000aa)]
    ports = [None, ('eq', 0), ('eq', 65535), ('neq', 22), ('lt', 1),
             ('gt', 65534), ('range', 22, 443), ('range', 0, 65535)]
    for index in range(count):
        rules = []
        for _ in range(rng.randint(1, 10)):
            proto = rng.choice([None, 6, 17, 1, 255])
            rules.append((rng.choice(['permit', 'deny']), proto, rng.choice(addresses),
                          rng.choice(addresses), rng.choice(ports) if proto in (6, 17) else None,
                          rng.choice(ports) if proto in (6, 17) else None))
        mode = index % 4
        changed = (list(rules) if mode == 0 else list(reversed(rules)) if mode == 1
                   else rules + [permit] if mode == 2 else [deny] + rules)
        yield rules, changed


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--seed', type=int, default=3304)
    parser.add_argument('--cases', type=int, default=160)
    parser.add_argument('--failure-dir', type=Path, default=Path('out/oracle-failure'))
    args = parser.parse_args()
    binary = args.binary.resolve()
    totals = {'EQUIVALENT': 0, 'DIFFERENT': 0}
    for index, (before, after) in enumerate(corpus(args.seed, args.cases)):
        with tempfile.TemporaryDirectory() as tmp:
            try:
                totals[check(binary, before, after, Path(tmp))] += 1
            except Exception:
                args.failure_dir.mkdir(parents=True, exist_ok=True)
                for file in Path(tmp).iterdir():
                    (args.failure_dir / file.name).write_bytes(file.read_bytes())
                (args.failure_dir / 'case.json').write_text(json.dumps({'seed': args.seed, 'index': index}))
                raise
    assert all(totals.values()), totals
    print(json.dumps({'seed': args.seed, 'z3': z3.get_version_string(), 'results': totals}))


if __name__ == '__main__':
    main()
