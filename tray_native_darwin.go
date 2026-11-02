package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
#include <stdlib.h>
int ecs_tray_start(void);
void ecs_tray_update(const char *json);
void ecs_tray_remove(void);
*/
import "C"

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"
	"unsafe"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type darwinTrayProfile struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Action   string `json:"action"`
	Label    string `json:"label"`
	Disabled bool   `json:"disabled"`
}
type darwinTrayMenu struct {
	Summary  string              `json:"summary"`
	Profiles []darwinTrayProfile `json:"profiles"`
	Running  bool                `json:"running"`
}

func darwinTraySnapshot(statuses []VPNStatus) darwinTrayMenu {
	menu := darwinTrayMenu{Summary: traySummary(statuses), Profiles: make([]darwinTrayProfile, 0, len(statuses))}
	for _, status := range statuses {
		label, action := trayLifecycleAction(status)
		menu.Profiles = append(menu.Profiles, darwinTrayProfile{ID: status.ProfileID, Title: status.ActiveProfile + " · " + trayState(status), Action: action.kind, Label: label, Disabled: status.State == "stopping"})
		menu.Running = menu.Running || status.Running
	}
	return menu
}

var darwinTray struct {
	sync.Mutex
	app     *App
	stop    chan struct{}
	refresh chan struct{}
}

func StartTray(app *App, _ context.Context) {
	if C.ecs_tray_start() == 0 {
		log.Printf("[VPN] macOS 菜单栏创建失败")
		return
	}
	darwinTray.Lock()
	darwinTray.app = app
	darwinTray.stop = make(chan struct{})
	darwinTray.refresh = make(chan struct{}, 1)
	stop, refresh := darwinTray.stop, darwinTray.refresh
	darwinTray.Unlock()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		var previous string
		for {
			data, err := json.Marshal(darwinTraySnapshot(app.trayStatuses()))
			if err != nil {
				log.Printf("[VPN] macOS 菜单栏更新失败: %v", err)
			} else if string(data) != previous {
				value := C.CString(string(data))
				C.ecs_tray_update(value)
				C.free(unsafe.Pointer(value))
				previous = string(data)
			}
			select {
			case <-stop:
				return
			case <-refresh:
			case <-ticker.C:
			}
		}
	}()
}

func UpdateTrayTooltip(string) {
	darwinTray.Lock()
	defer darwinTray.Unlock()
	if darwinTray.refresh != nil {
		select {
		case darwinTray.refresh <- struct{}{}:
		default:
		}
	}
}

func RemoveTrayIcon() {
	darwinTray.Lock()
	if darwinTray.stop != nil {
		close(darwinTray.stop)
		darwinTray.stop, darwinTray.refresh, darwinTray.app = nil, nil, nil
	}
	darwinTray.Unlock()
	C.ecs_tray_remove()
}

//export ecsDarwinTrayAction
func ecsDarwinTrayAction(kind, id *C.char) {
	action, profile := C.GoString(kind), C.GoString(id)
	darwinTray.Lock()
	app := darwinTray.app
	darwinTray.Unlock()
	if app == nil {
		return
	}
	go func() {
		app.mu.Lock()
		ctx, shuttingDown := app.ctx, app.shuttingDown
		app.mu.Unlock()
		if ctx == nil || shuttingDown {
			return
		}
		switch action {
		case "open":
			showTrayWorkspace(app)
		case "details":
			if app.SetActiveProfile(profile) {
				wailsruntime.EventsEmit(ctx, "profile-switched", profile)
				showTrayWorkspace(app)
			}
		case "quit":
			app.mu.Lock()
			if app.shuttingDown {
				app.mu.Unlock()
				return
			}
			app.shuttingDown = true
			app.mu.Unlock()
			wailsruntime.Quit(ctx)
		case "connect", "disconnect", "disconnect_all":
			result, cancelled := app.executeTrayAction(trayAction{kind: action, profileID: profile})
			if result != "OK" && !cancelled {
				_, _ = wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{Type: wailsruntime.ErrorDialog, Title: "VPN 操作失败", Message: result})
			}
		}
	}()
}
