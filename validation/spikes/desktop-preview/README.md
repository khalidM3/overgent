# Desktop preview feasibility spike

Status: PASS for a macOS preview, with a pinned-beta distribution narrow.

This spike pins Wails `v3.0.0-beta.12` and validates the smallest macOS shape
needed by Stickguy: one embedded native webview window, one persistent menu-bar
item, close-to-hide behavior, Open/Pause/Resume/Quit menu actions, and no
localhost listener. It contains synthetic status only and never opens the real
configuration, Keychain, repository, or hosted account.

Native macOS 27 arm64 validation proved embedded rendering, persistent
close-to-hide behavior, and the Open/Pause/Resume/Scan/Quit system-tray menu.
Pause changed the live menu status and label; Quit exited cleanly. The app had no
TCP listener, used 88,864 KiB RSS and 0.0% CPU at the idle observation point,
and produced a 15,232,050-byte executable.

Wails v2.15.0 is retained under `wails2/` as the stable window-only fallback. It
launched natively, but does not provide the required system-tray API. Therefore
ADR-029 accepts exact-pinned Wails v3 only for an explicitly labeled macOS
preview in a separate module. Signing, notarization, updater integration, and a
cross-platform release claim remain L8 work.
