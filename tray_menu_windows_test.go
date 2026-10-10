package main

import (
	"strings"
	"testing"
	"unicode/utf16"
	"unsafe"

	"EasyConnectSuite/engine"
)

func TestTraySummarySeparatesConnectedFromPendingAndFailed(t *testing.T) {
	statuses := []VPNStatus{
		{ProfileID: "selected", ActiveProfile: "未连接的当前配置", State: "disconnected"},
		{ProfileID: "a", ActiveProfile: "研发", Protocol: "topsec", State: "connected", Running: true},
		{ProfileID: "b", ActiveProfile: "办公", Protocol: "univpn", State: "connected", Running: true},
		{ProfileID: "c", ActiveProfile: "测试", State: "connecting", Running: true},
		{ProfileID: "d", ActiveProfile: "重连", State: "reconnecting", Running: true},
		{ProfileID: "e", ActiveProfile: "断开", State: "stopping", Running: true},
		{ProfileID: "f", ActiveProfile: "失败", State: "error"},
	}
	if got := traySummary(statuses); got != "2 条已连接 · 1 条连接中 · 1 条重连中 · 1 条断开中 · 1 条异常" {
		t.Fatal(got)
	}
	if tip := trayTooltip(statuses); !strings.Contains(tip, "研发：已连接") || !strings.Contains(tip, "办公：已连接") || strings.Contains(tip, "未连接的当前配置") {
		t.Fatal(tip)
	}
}

func TestTrayTooltipUTF16Bounds(t *testing.T) {
	for _, tip := range []string{"VPN 工作台\n办公🚀：已连接", strings.Repeat("名", 125) + "🚀结束", strings.Repeat("名", 126) + "🚀结束", strings.Repeat("🚀", 80)} {
		buffer := trayTooltipBuffer(tip)
		if buffer[127] != 0 {
			t.Fatal("missing tooltip terminator")
		}
		var units []uint16
		for _, u := range buffer {
			if u == 0 {
				break
			}
			units = append(units, u)
		}
		decoded := string(utf16.Decode(units))
		if strings.ContainsRune(decoded, '\uFFFD') || !strings.HasPrefix(tip, decoded) {
			t.Fatalf("split UTF-16 tooltip: %q", decoded)
		}
	}
}

func nativeMenuText(menu uintptr, index uintptr) string {
	var text [512]uint16
	user32.NewProc("GetMenuStringW").Call(menu, index, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)), 0x0400)
	var n int
	for n < len(text) && text[n] != 0 {
		n++
	}
	return string(utf16.Decode(text[:n]))
}

func TestNativeTrayMenuUsesEveryConnectionAndState(t *testing.T) {
	statuses := []VPNStatus{
		{ProfileID: "idle", ActiveProfile: "默认配置", Protocol: "easyconnect", State: "disconnected"},
		{ProfileID: "online", ActiveProfile: "研发 & 办公🚀", Protocol: "topsec", State: "connected", Running: true},
		{ProfileID: "pending", ActiveProfile: "测试", Protocol: "univpn", State: "connecting", Running: true},
		{ProfileID: "failed", ActiveProfile: "备用", State: "error"},
		{ProfileID: "stopping", ActiveProfile: "结束中", State: "stopping", Running: true},
	}
	menu, actions := buildTrayMenu(statuses)
	if menu == 0 {
		t.Fatal("native menu creation failed")
	}
	defer procDestroyMenu.Call(menu)
	getState := user32.NewProc("GetMenuState")
	getSubmenu := user32.NewProc("GetSubMenu")
	checked, _, _ := getState.Call(menu, 4, 0x0400)
	unchecked, _, _ := getState.Call(menu, 3, 0x0400)
	if checked&MF_CHECKED == 0 || unchecked&MF_CHECKED != 0 {
		t.Fatal("check mark follows selection rather than connection")
	}
	if text := nativeMenuText(menu, 4); text != "研发 && 办公🚀 · TopSAP · 已连接" {
		t.Fatal(text)
	}
	for i, expected := range []string{"连接此 VPN", "断开此 VPN", "取消连接", "重试连接", "断开此 VPN"} {
		submenu, _, _ := getSubmenu.Call(menu, uintptr(i+3))
		if text := nativeMenuText(submenu, 0); text != expected {
			t.Fatal(text)
		}
		if got := actions[uintptr(2000+i*2)]; got.profileID != statuses[i].ProfileID {
			t.Fatalf("wrong target: %+v", got)
		}
		if got := actions[uintptr(2001+i*2)]; got.kind != "details" || got.profileID != statuses[i].ProfileID {
			t.Fatalf("wrong detail target: %+v", got)
		}
		if i == 4 {
			state, _, _ := getState.Call(submenu, 0, 0x0400)
			if state&MF_GRAYED == 0 {
				t.Fatal("stopping action is still enabled")
			}
		}
	}
}

func TestTraySnapshotActionSurvivesSelectionAndProfileReorder(t *testing.T) {
	a := multiTestApp(t)
	engines := make(map[string]*fakeVPNEngine)
	start := func(o engine.Options) (vpnEngine, error) {
		e := &fakeVPNEngine{opts: o}
		engines[o.ConnectionID] = e
		o.OnStatus(engine.Status{Phase: engine.PhaseConnected})
		return e, nil
	}
	for _, id := range []string{"a", "b"} {
		if got := a.startConnectionID(id, start); got != "OK" {
			t.Fatal(got)
		}
	}
	defer a.StopAllVPN()
	menu, actions := buildTrayMenu(a.trayStatuses())
	defer procDestroyMenu.Call(menu)
	a.SetActiveProfile("b")
	a.mu.Lock()
	a.appConfig.Profiles[0], a.appConfig.Profiles[1] = a.appConfig.Profiles[1], a.appConfig.Profiles[0]
	a.mu.Unlock()
	if got := a.executeTrayAction(actions[2000]); got != "OK" {
		t.Fatal(got)
	}
	if !engines["a"].stopped.Load() || engines["b"].stopped.Load() || a.GetActiveProfile().ID != "b" {
		t.Fatal("tray operation crossed profiles or changed selection")
	}
	if got := a.executeTrayAction(trayAction{kind: "connect", profileID: "deleted"}); got != "服务器配置不存在" {
		t.Fatal(got)
	}
	if got := a.executeTrayAction(trayAction{kind: "disconnect", profileID: "deleted"}); got != "Not running" || engines["b"].stopped.Load() {
		t.Fatal("missing target fell back to selected connection")
	}
	if got := a.executeTrayAction(trayAction{kind: "disconnect_all"}); got != "OK" || !engines["b"].stopped.Load() {
		t.Fatal("disconnect all retained a session")
	}
}

func TestTrayStoppingSnapshotDoesNotChangePublicStatus(t *testing.T) {
	a := multiTestApp(t)
	a.connections = map[string]*vpnConnection{"a": {status: VPNStatus{ProfileID: "a", Running: true, State: "connected"}, stopping: true}}
	if snapshot := a.trayStatuses(); snapshot[0].State != "stopping" {
		t.Fatal(snapshot)
	}
	if a.GetConnections()[0].State != "connected" {
		t.Fatal("tray snapshot changed public state")
	}
}
