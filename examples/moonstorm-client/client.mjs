// Minimal external caller for the Plenilune analysis contract.
//
// It plays the role a future MoonStorm adapter would: it already holds both
// configuration versions in memory (MoonStorm owns access control, artifact
// retrieval/decryption and source identity), passes them to the Plenilune
// binary over stdin, and stores the returned artifact. It depends only on the
// binary and the JSON contract — no Plenilune packages, browser or UI.
//
// Usage: node client.mjs [path-to-plenilune-binary]
import { spawn } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../../', import.meta.url));
const binary = process.argv[2] ?? `${root}bin/plenilune${process.platform === 'win32' ? '.exe' : ''}`;
const fixture = (name) => readFileSync(`${root}testdata/configs/${name}`, 'utf8');

function invoke(args, input, { timeoutMs = 20000 } = {}) {
  return new Promise((resolve) => {
    const child = spawn(binary, args, { stdio: ['pipe', 'pipe', 'pipe'] });
    const out = [], err = [];
    const timer = setTimeout(() => child.kill(), timeoutMs);
    child.stdout.on('data', (d) => out.push(d));
    child.stderr.on('data', (d) => err.push(d));
    child.on('close', (code, signal) => {
      clearTimeout(timer);
      resolve({ code, signal, stdout: Buffer.concat(out).toString('utf8'), stderr: Buffer.concat(err).toString('utf8') });
    });
    child.stdin.end(input);
  });
}

const checks = [];
function check(name, ok, detail = '') {
  checks.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${detail ? ` — ${detail}` : ''}`);
}

// 1. Analyze two in-memory versions. Refs are caller labels, not identity.
const request = {
  schema: 'plenilune.analysis-request/v1',
  before: { text: fixture('regression/before.cfg'), ref: 'artifact://synthetic/edge-r1@v41' },
  after: { text: fixture('regression/after.cfg'), ref: 'artifact://synthetic/edge-r1@v42' },
  options: { acl: { limits: { max_operations: 2000000, max_boxes: 50000, timeout_ns: 2000000000 }, max_witnesses: 8 }, max_classes: 32, deadline_ms: 15000 },
};
const analyzed = await invoke(['analyze', '--request', '-'], JSON.stringify(request));
let result = null;
try { result = JSON.parse(analyzed.stdout); } catch { /* reported below */ }
check('analyze exits 0 with a plenilune.analysis/v2 artifact', analyzed.code === 0 && result?.schema === 'plenilune.analysis/v2' && /^[0-9a-f]{64}$/.test(result?.sha256 ?? ''), `exit ${analyzed.code}`);
check('engine and parser versions are pinned in the artifact', Boolean(result?.versions?.acl_engine && result?.versions?.prefix_list_engine && result?.versions?.intake));
check('caller refs are echoed for correlation only', result?.sources?.before?.ref === request.before.ref && result?.sources?.after?.ref === request.after.ref);
const mgmt = result?.objects?.find((o) => o.domain === 'acl' && o.name === 'MGMT');
const effects = mgmt?.acl?.changes?.classes?.map((c) => `${c.effect}:${c.witness.packet.dst_port}`) ?? [];
check('ACL change classes name what is newly denied and newly permitted', mgmt?.status === 'changed' && effects.join(',') === 'newly_denied:22,newly_permitted:3389', effects.join(','));
const customer = result?.objects?.find((o) => o.domain === 'prefix-list' && o.name === 'CUSTOMER');
check('second domain (prefix-list) is inventoried without ACL concepts', customer?.status === 'unchanged' && customer?.acl === undefined);
check('unanalyzed configuration is reported, not assumed unchanged', Array.isArray(result?.limitations) && result.limitations.includes('unanalyzed_configuration_present'));

// 2. Replay offline from the same bytes; no AI and no Plenilune UI involved.
const replay = await invoke(['analyze-replay', '--request', '-'], JSON.stringify({ schema: 'plenilune.analysis-replay-request/v1', before: request.before, after: request.after, evidence: result }));
check('analyze-replay reproduces the artifact', replay.code === 0 && JSON.parse(replay.stdout || '{}').status === 'ANALYSIS_REPLAYED', `exit ${replay.code}`);
const tampered = structuredClone(result);
tampered.objects.find((o) => o.name === 'MGMT').status = 'equivalent';
const forged = await invoke(['analyze-replay', '--request', '-'], JSON.stringify({ schema: 'plenilune.analysis-replay-request/v1', before: request.before, after: request.after, evidence: tampered }));
check('a tampered artifact is rejected with a typed error', forged.code === 2 && JSON.parse(forged.stderr || '{}').code === 'EVIDENCE_MISMATCH', forged.stderr.trim());

// 3. Typed request errors and cancellation never yield an artifact.
const bad = await invoke(['analyze', '--request', '-'], JSON.stringify({ ...request, schema: 'other/v1' }));
check('invalid request returns a typed JSON error and no stdout artifact', bad.code === 2 && bad.stdout === '' && JSON.parse(bad.stderr || '{}').code === 'REQUEST_SCHEMA', bad.stderr.trim());
const unknown = await invoke(['analyze', '--request', '-'], JSON.stringify({ ...request, tenant: 'x' }));
check('unknown request fields are rejected (strict contract)', unknown.code === 2 && JSON.parse(unknown.stderr || '{}').code === 'INVALID_JSON');
const canceled = await invoke(['analyze', '--request', '-'], JSON.stringify(request), { timeoutMs: 1 });
check('caller-side cancellation produces no artifact', canceled.stdout === '' || canceled.code === 0, `exit ${canceled.code} signal ${canceled.signal}`);

const failed = checks.filter((c) => !c.ok);
console.log(`\n${checks.length - failed.length}/${checks.length} contract checks passed`);
process.exitCode = failed.length ? 1 : 0;
