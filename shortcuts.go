package main

import (
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
)

type shortcutSlot struct {
	id          string
	accel       string
	description string
	run         func()
}

var (
	registeredShortcuts = map[string]string{}
	pendingRollback     = map[string]string{}
	pendingHasRollback  = map[string]bool{}
	pendingTimers       = map[string]*time.Timer{}
	pendingSeqs         = map[string]uint64{}
	shortcutsPaused     bool
)

func isWaylandSession() bool {
	return runtime.GOOS == "linux" && strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != ""
}

func shortcutSlots(a *app.App) []shortcutSlot {
	return []shortcutSlot{
		{
			id:          "search",
			accel:       a.Settings.EffectiveSearchShortcut(),
			description: locales.SettingsShortcutSearch(),
			run: func() {
				if a.Win.ShowQuickSearch != nil {
					a.Win.ShowQuickSearch()
				}
			},
		},
		{
			id:          "quickAdd",
			accel:       strings.TrimSpace(a.Settings.ShortcutQuickAdd),
			description: locales.SettingsShortcutQuickAdd(),
			run: func() {
				a.QuickAddFromClipboard()
			},
		},
		{
			id:          "notch",
			accel:       strings.TrimSpace(a.Settings.ShortcutNotch),
			description: locales.SettingsShortcutNotch(),
			run: func() {
				if a.Win.ToggleNotch != nil {
					a.Win.ToggleNotch()
				}
			},
		},
		{
			id:          "edge",
			accel:       strings.TrimSpace(a.Settings.ShortcutEdge),
			description: locales.SettingsShortcutEdge(),
			run: func() {
				if a.Win.ToggleEdge != nil {
					a.Win.ToggleEdge()
				}
			},
		},
	}
}

func setSlotAccel(s *model.Settings, id, accel string) {
	accel = strings.TrimSpace(accel)
	switch id {
	case "search":
		if strings.EqualFold(accel, model.DefaultSearchShortcut) {
			s.ShortcutSearch = ""
		} else {
			s.ShortcutSearch = accel
		}
		s.GlobalSearchOff = accel == ""
	case "quickAdd":
		s.ShortcutQuickAdd = accel
	case "notch":
		s.ShortcutNotch = accel
	case "edge":
		s.ShortcutEdge = accel
	}
}

func stopPendingTimer(slotID string) {
	if t := pendingTimers[slotID]; t != nil {
		t.Stop()
		delete(pendingTimers, slotID)
	}
}

func clearPendingRollback(slotID string) {
	stopPendingTimer(slotID)
	delete(pendingRollback, slotID)
	delete(pendingHasRollback, slotID)
}

func registerSlot(a *app.App, s shortcutSlot) error {
	slotID := s.id
	acc := s.accel
	fn := s.run
	stopPendingTimer(slotID)
	pendingSeqs[slotID]++
	seq := pendingSeqs[slotID]
	a.SetShortcutBoundKeys(slotID, "")
	a.SetShortcutPending(slotID, false)

	err := mygo.GlobalShortcut.RegisterWith(acc, mygo.GlobalShortcutOptions{
		Description: s.description,
		OnKeys: func(keys string) {
			if registeredShortcuts[slotID] != acc || pendingSeqs[slotID] != seq {
				return
			}
			stopPendingTimer(slotID)
			a.SetShortcutPending(slotID, false)
			if strings.TrimSpace(keys) == "" {
				mygo.GlobalShortcut.Unregister(acc)
				delete(registeredShortcuts, slotID)
				a.SetShortcutBoundKeys(slotID, "")
				if pendingHasRollback[slotID] {
					prev := pendingRollback[slotID]
					clearPendingRollback(slotID)
					setSlotAccel(&a.Settings, slotID, prev)
					a.SaveSettings()
				}
				a.SetShortcutErr(slotID, locales.SettingsShortcutRejected())
			} else {
				clearPendingRollback(slotID)
				a.SetShortcutBoundKeys(slotID, keys)
				a.SetShortcutErr(slotID, "")
			}
			a.Invalidate()
		},
	}, fn)
	if err != nil {
		return err
	}
	registeredShortcuts[slotID] = acc
	a.SetShortcutErr(slotID, "")
	if isWaylandSession() && a.ShortcutBoundKeys(slotID) == "" {
		a.SetShortcutPending(slotID, true)
		// Safety valve: if a Wayland portal never responds, unlock the UI after 15s.
		pendingTimers[slotID] = time.AfterFunc(15*time.Second, func() {
			a.Post(func() {
				if pendingSeqs[slotID] == seq && a.IsShortcutPending(slotID) {
					a.SetShortcutPending(slotID, false)
					delete(pendingTimers, slotID)
				}
			})
		})
	}
	return nil
}

func setGlobalShortcutsPaused(a *app.App, paused bool) {
	if paused && isWaylandSession() && a != nil && a.HasAnyShortcutPending() {
		return
	}
	if shortcutsPaused == paused {
		return
	}
	shortcutsPaused = paused
	if paused {
		clearGlobalShortcuts()
		return
	}
	applyGlobalShortcuts(a)
}

// trySetShortcut attempts to bind candidate to slot while preserving all other
// active slots. If registration fails (synchronously or via Wayland portal
// rejection), the slot's previous confirmed shortcut is restored and the error
// is recorded.
func trySetShortcut(a *app.App, slot, candidate string) bool {
	if a == nil {
		return false
	}
	// On Wayland, avoid overlapping BindShortcuts portal calls while any
	// slot's confirmation is still in flight (MyGo's portal dispatches replies
	// across the entire active registry).
	if isWaylandSession() && a.HasAnyShortcutPending() {
		shortcutsPaused = false
		applyGlobalShortcuts(a)
		return false
	}
	shortcutsPaused = false

	slots := shortcutSlots(a)
	var target *shortcutSlot
	for i := range slots {
		if slots[i].id == slot {
			target = &slots[i]
			break
		}
	}
	if target == nil {
		return false
	}

	// Ensure all non-target slots are registered first so duplicate bindings
	// across our own slots are also caught cleanly.
	clearGlobalShortcuts()
	for _, s := range slots {
		if s.id == slot || s.accel == "" {
			continue
		}
		if err := registerSlot(a, s); err != nil {
			a.SetShortcutErr(s.id, err.Error())
		}
	}

	candSlot := *target
	candSlot.accel = strings.TrimSpace(candidate)
	if candSlot.accel == "" {
		clearPendingRollback(slot)
		a.SetShortcutErr(slot, "")
		a.SetShortcutBoundKeys(slot, "")
		a.SetShortcutPending(slot, false)
		setSlotAccel(&a.Settings, slot, "")
		a.SaveSettings()
		return true
	}

	// Preserve the last confirmed binding as the rollback target; never
	// overwrite it with an unconfirmed intermediate value.
	if isWaylandSession() {
		if !pendingHasRollback[slot] && !strings.EqualFold(target.accel, candSlot.accel) {
			pendingRollback[slot] = target.accel
			pendingHasRollback[slot] = true
		}
	} else {
		clearPendingRollback(slot)
	}

	if err := registerSlot(a, candSlot); err != nil {
		prev := target.accel
		if pendingHasRollback[slot] {
			prev = pendingRollback[slot]
		}
		clearPendingRollback(slot)
		// Roll back: restore both OS binding and Settings to the last confirmed shortcut.
		if prev != "" {
			prevSlot := *target
			prevSlot.accel = prev
			_ = registerSlot(a, prevSlot)
		}
		if !strings.EqualFold(prev, target.accel) {
			setSlotAccel(&a.Settings, slot, prev)
			a.SaveSettings()
		}
		a.SetShortcutErr(slot, err.Error())
		a.Invalidate()
		return false
	}

	setSlotAccel(&a.Settings, slot, candSlot.accel)
	a.SaveSettings()
	return true
}

func applyGlobalShortcuts(a *app.App) {
	if a == nil || shortcutsPaused {
		return
	}
	slots := shortcutSlots(a)

	// Unregister any slots whose accelerator changed or was cleared first so
	// swapping keys between slots does not spuriously conflict.
	desired := make(map[string]string, len(slots))
	for _, s := range slots {
		desired[s.id] = s.accel
	}
	for id, prev := range registeredShortcuts {
		if prev == "" || desired[id] != prev {
			if prev != "" {
				mygo.GlobalShortcut.Unregister(prev)
			}
			delete(registeredShortcuts, id)
		}
	}

	for _, s := range slots {
		if s.accel == "" {
			clearPendingRollback(s.id)
			a.SetShortcutErr(s.id, "")
			a.SetShortcutBoundKeys(s.id, "")
			a.SetShortcutPending(s.id, false)
			continue
		}
		if registeredShortcuts[s.id] == s.accel && mygo.GlobalShortcut.IsRegistered(s.accel) {
			continue
		}
		if err := registerSlot(a, s); err != nil {
			clearPendingRollback(s.id)
			a.SetShortcutErr(s.id, err.Error())
		}
	}
}

func clearGlobalShortcuts() {
	for id := range pendingTimers {
		stopPendingTimer(id)
	}
	for id, acc := range registeredShortcuts {
		if acc != "" {
			mygo.GlobalShortcut.Unregister(acc)
		}
		delete(registeredShortcuts, id)
	}
	mygo.GlobalShortcut.UnregisterAll()
}
