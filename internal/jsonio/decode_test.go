package jsonio

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type safeDomain string

func (*safeDomain) UnmarshalJSON([]byte) error { return Invalid("fixed domain diagnostic") }

type unsafeDomain string

func (*unsafeDomain) UnmarshalJSON(data []byte) error {
	return errors.New("bad input: " + string(data))
}

func TestOnlyTrustedDomainDiagnosticsPassThrough(t *testing.T) {
	var safe struct {
		Value safeDomain `json:"value"`
	}
	err := Decode([]byte(`{"value":"SECRET"}`), 1024, &safe)
	var domain *ValidationError
	if !errors.As(err, &domain) || domain.Error() != "fixed domain diagnostic" {
		t.Fatalf("domain diagnostic lost: %v", err)
	}
	var unsafe struct {
		Value unsafeDomain `json:"value"`
	}
	err = Decode([]byte(`{"value":"SECRET"}`), 1024, &unsafe)
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("raw decode input leaked: %v", err)
	}
}

func FuzzDecode(f *testing.F) {
	for _, seed := range []string{`{"name":"ok"}`, `{"name":"a","Name":"b"}`, `{"name":null}`, `{"name":"a","name":"b"}`, `[]`, `null`, `{"name":"\ud800"}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var got struct {
			Name string `json:"name"`
		}
		if err := Decode(data, 4096, &got); err != nil {
			return
		}
		b, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var replay struct {
			Name string `json:"name"`
		}
		if err := Decode(b, 4096, &replay); err != nil || !reflect.DeepEqual(got, replay) {
			t.Fatalf("accepted JSON failed typed roundtrip: %v", err)
		}
	})
}

func TestAmbiguousAndUnexpectedInputRejected(t *testing.T) {
	for _, input := range []string{`{"name":"a","name":"b"}`, `{"name":"a","\u006eame":"b"}`, `{"name":"a","Name":"b"}`, `{"Name":"a"}`, `{"other":1}`, `{"name":"a"} {}`, `{"name":1}`, string([]byte{0xff})} {
		var dst struct {
			Name string `json:"name"`
		}
		if err := Decode([]byte(input), 1024, &dst); err == nil {
			t.Errorf("accepted invalid contract %q", input)
		}
	}
}

func TestExactNestedFieldSpelling(t *testing.T) {
	var dst struct {
		Items []struct {
			Value int `json:"value"`
		} `json:"items"`
	}
	if err := Decode([]byte(`{"items":[{"value":1,"Value":2}]}`), 1024, &dst); err == nil {
		t.Fatal("nested field alias accepted")
	}
}

func TestDecodeLimitsAndValidInput(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	if err := Decode([]byte(`{"name":"ok"}`), 1024, &dst); err != nil || dst.Name != "ok" {
		t.Fatalf("decode: %v", err)
	}
	if err := Decode([]byte(`{"name":"ok"}`), 2, &dst); err == nil {
		t.Fatal("size limit ignored")
	}
}
