//go:build linux || windows

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// singleInstance keeps one Overgent per member and routes a second launch's
// deep link into it.
//
// macOS does not need this: Launch Services re-activates the running
// application for an overgent:// link, and Wails emits the launch-with-URL
// event again in the process that is already up. Linux and Windows do the
// opposite - the desktop entry and the registry command both start a new
// process with the URL as an argument - so without this a link would open a
// second window with a second tray icon, and the local service would be talked
// to by two shells that disagree about what is on screen.
//
// The second process exits after handing its arguments over. Only the URL is
// forwarded; the working directory and everything else Wails offers are
// deliberately ignored, because a link is attacker-reachable and the argument
// list is the part that is already validated.
func singleInstance(deliver func(string)) *application.SingleInstanceOptions {
	return &application.SingleInstanceOptions{
		UniqueID: desktopApplicationID(),
		OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
			if raw, ok := deepLinkFromArguments(data.Args); ok {
				deliver(raw)
			}
		},
	}
}
