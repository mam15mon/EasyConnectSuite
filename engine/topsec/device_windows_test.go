package topsec

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func TestDeviceEnumerationAfterShortVirtualPNP(t *testing.T) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\EasyConnectSuiteDeviceTest`, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.DeleteKey(registry.CURRENT_USER, `Software\EasyConnectSuiteDeviceTest`)
	defer key.Close()
	values := []string{"ROOT\\NET\\0000", "PCI\\VEN_1234&DEV_5678\\" + strings.Repeat("A", 160), "PCI\\VEN_5678\\SECOND"}
	for i, value := range values {
		if err := key.SetStringValue(string(rune('a'+i)), value); err != nil {
			t.Fatal(err)
		}
	}
	adapters := make([]windows.IpAdapterInfo, 4)
	for i := range adapters {
		adapters[i].AdapterName[0] = byte('a' + i)
		adapters[i].AddressLength = 6
		adapters[i].Address = [8]byte{2, 0, 0, 0, 0, byte(i + 1)}
		if i+1 < len(adapters) {
			adapters[i].Next = &adapters[i+1]
		}
	}
	var output bytes.Buffer
	original := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(original)
	read := func(name string) (string, error) {
		value, _, err := key.GetStringValue(name)
		return value, err
	}
	got, err := deviceCodeFromAdapters(&adapters[0], "synthetic-profile", read)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := deviceCode(adapters[1].Address[:6], adapters[2].Address[:6])
	if got != want {
		t.Fatal("PCI adapters after short virtual PNP must retain enumeration order")
	}
	for _, text := range []string{`profile="synthetic-profile"`, "pci=false", "selected=true", "skipped=", "adapters=4 selected=2"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("missing diagnostic %q", text)
		}
	}
	for _, secret := range append(values, got, "02:00:00:00:00:02") {
		if strings.Contains(output.String(), secret) {
			t.Fatal("hardware identity leaked to logs")
		}
	}
}

func TestDeviceEnumerationWithoutEligibleAdapter(t *testing.T) {
	adapter := &windows.IpAdapterInfo{AddressLength: 6}
	for _, read := range []func(string) (string, error){
		func(string) (string, error) { return "ROOT\\NET\\0000", nil },
		func(string) (string, error) { return "", errors.New("read_pnp: access denied") },
	} {
		if _, err := deviceCodeFromAdapters(adapter, "synthetic-profile", read); err == nil {
			t.Fatal("no eligible PCI adapter must fail without generating another identity")
		}
	}
}
