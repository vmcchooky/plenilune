package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"plenilune/internal/evidence"
	"plenilune/internal/jsonio"
	"strings"
	"testing"
)

func invoke(args ...string) (int, string, string) {
	var out, err bytes.Buffer
	code := Run(context.Background(), args, &out, &err)
	return code, out.String(), err.String()
}

func put(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTraceReplayAndModifiedInputs(t *testing.T) {
	dir := t.TempDir()
	config := put(t, dir, "acl.cfg", "ip access-list extended DEMO\n permit tcp host 192.0.2.10 host 198.51.100.20 eq 22\n deny ip any any\n")
	packet := put(t, dir, "packet.json", `{"protocol":6,"src_ip":"192.0.2.10","dst_ip":"198.51.100.20","src_port":40000,"dst_port":22}`)
	code, body, errout := invoke("trace", "--config", config, "--packet", packet)
	if code != 0 || errout != "" {
		t.Fatalf("trace: %d %s", code, errout)
	}
	var trace evidence.Trace
	if err := jsonio.Decode([]byte(body), evidence.MaxTraceBytes, &trace); err != nil {
		t.Fatal(err)
	}
	if trace.Decision.Action != "permit" || trace.Decision.Implicit {
		t.Fatal("wrong packet decision")
	}
	proof := put(t, dir, "trace.json", body)
	code, body, errout = invoke("verify", "--config", config, "--evidence", proof)
	if code != 0 || !strings.Contains(body, "TRACE_REPLAYED") {
		t.Fatalf("replay: %d %s", code, errout)
	}
	// Even a self-consistently rehashed false decision must fail semantic replay.
	trace.Decision.Action = "deny"
	tampered, err := evidence.SealTrace(trace.ConfigSHA256, trace.Packet, trace.Decision)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	put(t, dir, "trace.json", string(b))
	if code, _, _ := invoke("verify", "--config", config, "--evidence", proof); code != 2 {
		t.Fatal("accepted rehashed false decision")
	}
	put(t, dir, "trace.json", body) // invalid trace shape must also fail.
	if code, _, _ := invoke("verify", "--config", config, "--evidence", proof); code != 2 {
		t.Fatal("accepted non-trace report")
	}
}

func TestOriginalBytesRequiredAndNoPartialOutput(t *testing.T) {
	dir := t.TempDir()
	config := put(t, dir, "acl.cfg", "ip access-list extended DEMO\n deny ip any any\n")
	packet := put(t, dir, "packet.json", `{"protocol":6,"src_ip":"192.0.2.10","dst_ip":"198.51.100.20","src_port":40000,"dst_port":22}`)
	code, body, errout := invoke("trace", "--config", config, "--packet", packet)
	if code != 0 {
		t.Fatalf("trace: %s", errout)
	}
	proof := put(t, dir, "trace.json", body)
	put(t, dir, "acl.cfg", "ip access-list extended DEMO\n remark different bytes\n deny ip any any\n")
	if code, out, _ := invoke("verify", "--config", config, "--evidence", proof); code != 2 || out != "" {
		t.Fatal("accepted altered source or emitted partial result")
	}
	put(t, dir, "packet.json", `{"protocol":6,"protocol":17,"src_ip":"192.0.2.1","dst_ip":"198.51.100.1","src_port":1,"dst_port":22}`)
	if code, out, _ := invoke("trace", "--config", config, "--packet", packet); code != 2 || out != "" {
		t.Fatal("accepted duplicate packet key")
	}
	put(t, dir, "packet.json", `{"src_ip":"192.0.2.1","dst_ip":"198.51.100.1","src_port":1,"dst_port":22}`)
	if code, _, _ := invoke("trace", "--config", config, "--packet", packet); code != 2 {
		t.Fatal("accepted missing protocol")
	}
}

func TestUsageVersionCancellation(t *testing.T) {
	for _, args := range [][]string{{"diff"}, {"trace", "--config", "missing"}, {"version", "--ai=on"}, {"parse", "--config", "missing", "extra"}} {
		if code, out, _ := invoke(args...); code != 2 || out != "" {
			t.Fatalf("bad usage accepted: %v", args)
		}
	}
	if code, out, _ := invoke("version"); code != 0 || !strings.Contains(out, "unversioned") && !strings.Contains(out, "commit") {
		t.Fatal("version missing build provenance")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, err bytes.Buffer
	if Run(ctx, []string{"version"}, &out, &err) != 2 || out.Len() != 0 {
		t.Fatal("canceled invocation produced output")
	}
}

func TestEvidencePacketsRejectNullMissingAndAliasFields(t *testing.T) {
	dir := t.TempDir()
	config := put(t, dir, "acl.cfg", "ip access-list extended DEMO\n deny ip any any\n")
	packet := put(t, dir, "packet.json", `{"protocol":0,"src_ip":"192.0.2.1","dst_ip":"198.51.100.1","src_port":0,"dst_port":0}`)
	code, body, errout := invoke("trace", "--config", config, "--packet", packet)
	if code != 0 {
		t.Fatalf("trace: %s", errout)
	}
	for _, modified := range []string{
		strings.Replace(body, `"protocol": 0`, `"protocol": null`, 1),
		strings.Replace(body, `"protocol": 0,`, "", 1),
		strings.Replace(body, `"src_port": 0`, `"src_port": null`, 1),
		strings.Replace(body, `"dst_port": 0`, `"dst_port": null`, 1),
		strings.Replace(body, `"protocol": 0`, `"protocol": 0, "Protocol": 17`, 1),
	} {
		proof := put(t, dir, "trace.json", modified)
		if code, out, _ := invoke("verify", "--config", config, "--evidence", proof); code != 2 || out != "" {
			t.Fatal("accepted invalid embedded packet")
		}
	}
}

func TestObligationsCLIAndDomainDiagnostics(t *testing.T) {
	data, err := os.ReadFile("../../testdata/obligations.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	conditions := put(t, dir, "conditions.json", string(data))
	code, out, errout := invoke("obligations", "--file", conditions)
	if code != 0 || errout != "" || !strings.Contains(out, "VALIDATED_NOT_EVALUATED") {
		t.Fatalf("obligations: %d %s", code, errout)
	}
	bad := strings.Replace(string(data), `"hi": 22`, `"hi": null`, 1)
	if bad == string(data) {
		t.Fatal("fixture no longer exercises hi bound")
	}
	put(t, dir, "conditions.json", bad)
	code, out, errout = invoke("obligations", "--file", conditions)
	if code != 2 || out != "" || !strings.Contains(errout, "INVALID_OBLIGATION") || !strings.Contains(errout, "requires lo and hi") {
		t.Fatalf("semantic error lost: %d %s", code, errout)
	}
	config := put(t, dir, "acl.cfg", "ip access-list extended DEMO\n deny ip any any\n")
	packet := put(t, dir, "packet.json", `{"protocol":47,"src_ip":"192.0.2.1","dst_ip":"198.51.100.1","src_port":1,"dst_port":0}`)
	code, out, errout = invoke("trace", "--config", config, "--packet", packet)
	if code != 2 || out != "" || !strings.Contains(errout, "ports must be zero") {
		t.Fatalf("packet diagnostic lost: %d %s", code, errout)
	}
	put(t, dir, "packet.json", `{"protocol":6,"src_ip":"SENSITIVE_INPUT","dst_ip":"198.51.100.1","src_port":1,"dst_port":22}`)
	code, _, errout = invoke("trace", "--config", config, "--packet", packet)
	if code != 2 || strings.Contains(errout, "SENSITIVE_INPUT") {
		t.Fatal("raw decode input leaked")
	}
}

func TestReplayRejectsMissingOrNullDecisionFlag(t *testing.T) {
	dir := t.TempDir()
	config := put(t, dir, "acl.cfg", "ip access-list extended DEMO\n deny ip any any\n")
	packet := put(t, dir, "packet.json", `{"protocol":6,"src_ip":"192.0.2.1","dst_ip":"198.51.100.1","src_port":1,"dst_port":22}`)
	code, body, errout := invoke("trace", "--config", config, "--packet", packet)
	if code != 0 {
		t.Fatal(errout)
	}
	for _, modified := range []string{strings.Replace(body, `"implicit": false`, `"implicit": null`, 1), strings.Replace(body, `"implicit": false`, "", 1)} {
		// Remove a leading comma for the omitted-field variant to stay valid JSON.
		modified = strings.Replace(modified, ",\n    \n", "\n", 1)
		if !json.Valid([]byte(modified)) {
			t.Fatal("test must reach semantic validation with well-formed JSON")
		}
		proof := put(t, dir, "trace.json", modified)
		if code, out, _ := invoke("verify", "--config", config, "--evidence", proof); code != 2 || out != "" {
			t.Fatal("accepted unspecified decision flag")
		}
	}
}
