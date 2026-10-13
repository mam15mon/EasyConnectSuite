package easyconnect

import (
	"reflect"
	"testing"
)

func TestParseLineListRejectsInvalidConfig(t *testing.T) {
	for _, config := range []string{
		"<html>session expired</html>", "", "<Conf/>",
		"<Conf><Mline list=\"vpn.example:443\"/></Conf>",
		"<Conf><Mline enable=\"0\" list=\"vpn.example:443\"/></Conf>",
		"<Conf><Mline enable=\"1\"/></Conf>",
	} {
		t.Run(config, func(t *testing.T) {
			c := &Client{}
			if err := c.parseLineListFromConfig(config); err == nil {
				t.Fatal("invalid configuration was accepted")
			}
			if len(c.lineList) != 0 {
				t.Fatal("invalid configuration installed gateway lines")
			}
		})
	}
}

func TestParseLineListAcceptsEnabledConfig(t *testing.T) {
	c := &Client{}
	if err := c.parseLineListFromConfig("<Conf><Mline enable=\"1\" list=\"one.example:443;two.example:443;\"/></Conf>"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.lineList, []string{"one.example:443", "two.example:443"}) {
		t.Fatalf("line list = %v", c.lineList)
	}
}
