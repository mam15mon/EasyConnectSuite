package main

import (
	"fmt"
	"strings"
	"unicode/utf16"
)

type trayAction struct {
	kind      string
	profileID string
}

func (a *App) trayStatusesLocked() []VPNStatus {
	statuses := a.connectionStatusesLocked()
	for i := range statuses {
		if c := a.connections[statuses[i].ProfileID]; c != nil && c.stopping {
			statuses[i].State = "stopping"
		}
	}
	return statuses
}

func (a *App) trayStatuses() []VPNStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.trayStatusesLocked()
}

func trayState(s VPNStatus) string {
	if s.State == "stopping" {
		return "断开中"
	}
	if s.Running {
		switch s.State {
		case "connected":
			return "已连接"
		case "reconnecting":
			return "重连中"
		default:
			return "连接中"
		}
	}
	if s.State == "error" {
		return "连接出错"
	}
	return "未连接"
}

func traySummary(statuses []VPNStatus) string {
	var connected, connecting, reconnecting, stopping, failed int
	for _, s := range statuses {
		switch trayState(s) {
		case "已连接":
			connected++
		case "连接中":
			connecting++
		case "重连中":
			reconnecting++
		case "断开中":
			stopping++
		case "连接出错":
			failed++
		}
	}
	parts := []string{fmt.Sprintf("%d 条已连接", connected)}
	for _, count := range []struct {
		n     int
		label string
	}{{connecting, "连接中"}, {reconnecting, "重连中"}, {stopping, "断开中"}, {failed, "异常"}} {
		if count.n > 0 {
			parts = append(parts, fmt.Sprintf("%d 条%s", count.n, count.label))
		}
	}
	return strings.Join(parts, " · ")
}

func trayTooltip(statuses []VPNStatus) string {
	lines := []string{"VPN 工作台", traySummary(statuses)}
	for _, s := range statuses {
		if s.Running || s.State == "error" {
			lines = append(lines, fmt.Sprintf("%s：%s", strings.ReplaceAll(s.ActiveProfile, "\n", " "), trayState(s)))
		}
	}
	return strings.Join(lines, "\n")
}

func trayTooltipBuffer(tip string) [128]uint16 {
	var buffer [128]uint16
	units := utf16.Encode([]rune(tip))
	if len(units) > 127 {
		units = units[:127]
		if utf16.IsSurrogate(rune(units[126])) && units[126] < 0xDC00 {
			units = units[:126]
		}
	}
	copy(buffer[:], units)
	return buffer
}

func trayLifecycleAction(s VPNStatus) (string, trayAction) {
	action := trayAction{profileID: s.ProfileID, kind: "connect"}
	if s.Running || s.State == "stopping" {
		action.kind = "disconnect"
		if s.State == "connecting" {
			return "取消连接", action
		}
		return "断开此 VPN", action
	}
	if s.State == "error" {
		return "重试连接", action
	}
	return "连接此 VPN", action
}

func (a *App) executeTrayAction(action trayAction) string {
	switch action.kind {
	case "connect":
		return a.StartProfileVPN(action.profileID)
	case "disconnect":
		return a.StopProfileVPN(action.profileID)
	case "disconnect_all":
		return a.StopAllVPN()
	default:
		return "未知托盘操作"
	}
}
