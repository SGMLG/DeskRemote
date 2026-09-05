//go:build windows
package main

import (
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const runRegistryKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const appRegistryName = "DeskRemote"

func setAutostart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runRegistryKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if enable {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}
		cmdLine := `"` + exePath + `"`
		if len(os.Args) > 1 {
			for _, arg := range os.Args[1:] {
				if strings.Contains(arg, " ") || strings.Contains(arg, `"`) {
					escapedArg := strings.ReplaceAll(arg, `"`, `\"`)
					cmdLine += ` "` + escapedArg + `"`
				} else {
					cmdLine += " " + arg
				}
			}
		}
		return k.SetStringValue(appRegistryName, cmdLine)
	}
	return k.DeleteValue(appRegistryName)
}

func isAutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	_, _, err = k.GetStringValue(appRegistryName)
	return err == nil
}
