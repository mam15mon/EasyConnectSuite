package topsec

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"testing"
)

func TestLiteralProtocolVectors(t *testing.T) {
	code, err := deviceCode([]byte{2, 0xab, 0, 1, 2, 3})
	if err != nil || code != "tOn2cYFE+wqTmNHkz2AA7g==" {
		t.Fatal("device identifier differs from independent vector", err)
	}
	if _, err := deviceCode([]byte{1}); err == nil {
		t.Fatal("invalid MAC accepted")
	}
	want, _ := hex.DecodeString("000000000000000700000008000000010000000000000000")
	if !bytes.Equal(frame(7, make([]byte, 8)), want) {
		t.Fatal("incorrect initial command 7")
	}
	ack, _ := hex.DecodeString("00000000000000020000000000000001")
	command, body, err := readFrame(bytes.NewReader(ack))
	if err != nil || command != 2 || len(body) != 0 {
		t.Fatal("invalid setup ACK decode", err)
	}
	packet, _ := hex.DecodeString("0000aabb0000000f0000001c eeff0800 4500001c0000000040110000 c0000201c6336401 04d210e100080000")
	// DecodeString requires no spaces; keep a separate literal for readability.
	packet, _ = hex.DecodeString("0000aabb0000000f0000001ceeff08004500001c0000000040110000c0000201c633640104d210e100080000")
	command, body, err = readFrame(bytes.NewReader(packet))
	if err != nil || command != 15 || len(body) != 28 || body[9] != 17 {
		t.Fatal("UDP IPv4 literal decode failed", err)
	}
	for _, index := range []int{0, 12, 16, 18} {
		bad := append([]byte(nil), packet...)
		bad[index] ^= 1
		if _, _, err := readFrame(bytes.NewReader(bad)); err == nil {
			t.Fatalf("corrupt byte %d accepted", index)
		}
	}
	if _, _, err := readFrame(bytes.NewReader(packet[:17])); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("truncated frame accepted", err)
	}
}
func TestGatewayObjectDecoderDoesNotEvaluateContent(t *testing.T) {
	var value struct {
		Text   string `json:"text"`
		Values []bool `json:"values"`
		Ret    *int   `json:"Ret"`
	}
	if err := decodeObject([]byte(`{text:"a,b:{x}\\\"", values:[true,false], Ret:0}`), &value); err != nil || value.Ret == nil || len(value.Values) != 2 || value.Values[1] {
		t.Fatal("decoder changed quoted or array content", err)
	}
	for _, text := range []string{`{Ret: function(){return 0}()}`, `{Ret:0};process.exit()`, `{Ret:0, text:undefined}`} {
		if err := decodeObject([]byte(text), &value); err == nil {
			t.Fatal("executable/non-JSON input accepted")
		}
	}
	var missing reply
	if err := decodeObject([]byte(`{ErrMsg:"unknown"}`), &missing); err != nil {
		t.Fatal(err)
	}
	if err := missing.check(); err == nil {
		t.Fatal("missing Ret treated as success")
	}
}
func TestResourcesPreservePrefixesAndRejectUnknownSchemas(t *testing.T) {
	var r reply
	if err := decodeObject([]byte(`{"Ret":0,"res_list":[{"module":"na","protocol":"ip","ip":"198.51.100.19","mask":"255.255.255.0","domain":"","port":""}],"dns_info":{"dns_list":"","domain_list":""},"sec_rule":{"num":0}}`), &r); err != nil {
		t.Fatal(err)
	}
	routes, err := parseResources(r)
	if err != nil || len(routes) != 1 || routes[0].String() != "198.51.100.0/24" {
		t.Fatal("resource prefix changed", err)
	}
	r.Resources[0].Mask = "255.0.255.0"
	if _, err := parseResources(r); err == nil {
		t.Fatal("noncontiguous mask accepted")
	}
	r.Resources[0].Mask = "255.255.255.0"
	r.DNS.Domains = "example.invalid"
	if _, err := parseResources(r); err == nil {
		t.Fatal("unknown DNS schema accepted")
	}
	r.DNS.Domains = ""
	r.Resources = nil
	if _, err := parseResources(r); err == nil {
		t.Fatal("missing resources became unrestricted routing")
	}
}
