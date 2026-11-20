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

const portalConfirmTimeout = 30 * time.Second

type shortcutSlot struct {
	id          string
	accel       string
	description string
	run         func()
}

type pendingShortcutEdit struct {
	slot      string
	candidate string
}

var (
	registeredShortcuts  = map[string]string{}
	pendingTargets       = map[string]string{}
	pendingRollback      = map[string]string{}
	pendingHasRollback   = map[string]bool{}
	pendingSeqs          = map[string]uint64{}
	timedOutReplyKeys    = map[string]string{}
	timedOutCandidates   = map[string]bool{}
	deferredEdit         *pendingShortcutEdit
	portalTimer          *time.Timer
	portalTimerSeq       uint64
	portalTimedOut       bool
	portalDrainScheduled bool
	portalObserved       bool
	shortcutsPaused      bool
)

// isPureWayland reports whether Linux is running natively on Wayland without
// an X11 display fallback (so MyGo always uses the GlobalShortcuts portal).
func isPureWayland() bool {
	if runtime.GOOS != "linux" || strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GDK_BACKEND")), "wayland") {
		return true
	}
	return strings.TrimSpace(os.Getenv("DISPLAY")) == ""
}

// usesShortcutPortal reports whether global shortcuts go through the async
// Wayland GlobalShortcuts portal (where OnKeys fires asynchronously), matching
// MyGo's own backend selection across pure Wayland, XWayland-with-portal,
// XWayland-without-portal, X11, macOS, and Windows.
func usesShortcutPortal() bool {
	return runtime.GOOS == "linux" && (portalObserved || isPureWayland() || hasLinuxShortcutPortal())
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

func slotAccel(a *app.App, id string) string {
	switch id {
	case "search":
		return a.Settings.EffectiveSearchShortcut()
	case "quickAdd":
		return strings.TrimSpace(a.Settings.ShortcutQuickAdd)
	case "notch":
		return strings.TrimSpace(a.Settings.ShortcutNotch)
	case "edge":
		return strings.TrimSpace(a.Settings.ShortcutEdge)
	default:
		return ""
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

func clearSlotPendingMeta(slotID string) {
	delete(pendingTargets, slotID)
	delete(pendingRollback, slotID)
	delete(pendingHasRollback, slotID)
}

func stopPortalTimer() {
	if portalTimer != nil {
		portalTimer.Stop()
		portalTimer = nil
	}
}

func shortcutsMatchSettings(a *app.App) bool {
	if a == nil {
		return false
	}
	slots := shortcutSlots(a)
	nonEmpty := 0
	for _, s := range slots {
		if s.accel == "" {
			if _, ok := registeredShortcuts[s.id]; ok {
				return false
			}
			continue
		}
		nonEmpty++
		if registeredShortcuts[s.id] != s.accel {
			return false
		}
	}
	return len(registeredShortcuts) == nonEmpty
}

// finishTimedOutDrain runs after a timed-out Wayland portal request finally
// completes (when MyGo's internal p.busy is false and p.stale is false). It
// either executes a queued user edit with fresh IDs, confirms matching slots
// in place, or resyncs the OS back to the confirmed bindings in a.Settings.
func finishTimedOutDrain(a *app.App) {
	if !portalTimedOut {
		portalDrainScheduled = false
		return
	}
	portalTimedOut = false
	portalDrainScheduled = false
	replies := timedOutReplyKeys
	timedOutReplyKeys = map[string]string{}
	prevCandidates := timedOutCandidates
	timedOutCandidates = map[string]bool{}
	edit := deferredEdit
	deferredEdit = nil

	if edit != nil {
		stopPortalTimer()
		a.SetShortcutPending(edit.slot, false)
		trySetShortcut(a, edit.slot, edit.candidate)
		return
	}

	if shortcutsMatchSettings(a) {
		for id, keys := range replies {
			if strings.TrimSpace(keys) != "" {
				a.SetShortcutBoundKeys(id, keys)
				a.SetShortcutErr(id, "")
			} else {
				a.SetShortcutBoundKeys(id, "")
				a.SetShortcutErr(id, locales.SettingsShortcutRejected())
			}
		}
		a.Invalidate()
		return
	}

	clearGlobalShortcuts()
	applyGlobalShortcuts(a)
	for id := range prevCandidates {
		if slotAccel(a, id) == "" {
			a.SetShortcutErr(id, locales.SettingsShortcutRejected())
		}
	}
	a.Invalidate()
}

// armPortalTimer guards against a slow or unresponsive desktop portal daemon
// so Settings shortcut rows can never be locked permanently.
func armPortalTimer(a *app.App) {
	if a == nil || !usesShortcutPortal() {
		return
	}
	stopPortalTimer()
	portalTimerSeq++
	seq := portalTimerSeq
	portalTimer = time.AfterFunc(portalConfirmTimeout, func() {
		a.Post(func() {
			if portalTimerSeq != seq || !a.HasAnyShortcutPending() {
				return
			}
			portalTimer = nil
			if len(registeredShortcuts) > 0 {
				portalTimedOut = true
			}
			if deferredEdit != nil {
				slot := deferredEdit.slot
				deferredEdit = nil
				a.SetShortcutPending(slot, false)
				a.SetShortcutErr(slot, locales.SettingsShortcutRejected())
			}
			for id := range a.ShortcutPending {
				a.SetShortcutPending(id, false)
				if _, wasChange := pendingTargets[id]; wasChange {
					delete(pendingTargets, id)
					timedOutCandidates[id] = true
					a.SetShortcutErr(id, locales.SettingsShortcutRejected())
				}
			}
			a.Invalidate()
		})
	})
}

func registerSlot(a *app.App, s shortcutSlot) error {
	slotID := s.id
	acc := s.accel
	fn := s.run
	pendingSeqs[slotID]++
	seq := pendingSeqs[slotID]
	a.SetShortcutBoundKeys(slotID, "")
	a.SetShortcutPending(slotID, false)

	err := mygo.GlobalShortcut.RegisterWith(acc, mygo.GlobalShortcutOptions{
		Description: s.description,
		OnKeys: func(keys string) {
			portalObserved = true
			if registeredShortcuts[slotID] != acc || pendingSeqs[slotID] != seq {
				return
			}
			// While a timed-out portal request is draining, wait for the whole
			// reportShortcuts pass to finish (so MyGo's p.busy is false) and
			// then run any queued edit or resync cleanly on the next tick.
			if portalTimedOut {
				timedOutReplyKeys[slotID] = keys
				if !portalDrainScheduled {
					portalDrainScheduled = true
					a.Post(func() {
						finishTimedOutDrain(a)
					})
				}
				return
			}
			trimmedKeys := strings.TrimSpace(keys)
			a.SetShortcutPending(slotID, false)
			if !a.HasAnyShortcutPending() {
				stopPortalTimer()
			}

			pendingAcc, hasPendingTarget := pendingTargets[slotID]
			matchesPendingTarget := hasPendingTarget && pendingAcc == acc
			hasRollback := pendingHasRollback[slotID]
			rollbackAcc := pendingRollback[slotID]
			if matchesPendingTarget || hasRollback {
				delete(pendingTargets, slotID)
				delete(pendingRollback, slotID)
				delete(pendingHasRollback, slotID)
			}

			if trimmedKeys == "" {
				a.SetShortcutBoundKeys(slotID, "")
				if matchesPendingTarget {
					mygo.GlobalShortcut.Unregister(acc)
					delete(registeredShortcuts, slotID)
					if usesShortcutPortal() && len(registeredShortcuts) > 0 {
						clearGlobalShortcuts()
					}
					applyGlobalShortcuts(a)
				} else if hasRollback {
					mygo.GlobalShortcut.Unregister(acc)
					delete(registeredShortcuts, slotID)
					setSlotAccel(&a.Settings, slotID, rollbackAcc)
					if usesShortcutPortal() && len(registeredShortcuts) > 0 {
						clearGlobalShortcuts()
					}
					a.SaveSettings()
				} else if !usesShortcutPortal() {
					mygo.GlobalShortcut.Unregister(acc)
					delete(registeredShortcuts, slotID)
				}
				a.SetShortcutErr(slotID, locales.SettingsShortcutRejected())
			} else {
				a.SetShortcutBoundKeys(slotID, keys)
				a.SetShortcutErr(slotID, "")
				if matchesPendingTarget {
					setSlotAccel(&a.Settings, slotID, pendingAcc)
					a.SaveSettings()
				}
			}
			a.Invalidate()
		},
	}, fn)
	if err != nil {
		return err
	}
	registeredShortcuts[slotID] = acc
	a.SetShortcutErr(slotID, "")
	if usesShortcutPortal() && a.ShortcutBoundKeys(slotID) == "" {
		a.SetShortcutPending(slotID, true)
		armPortalTimer(a)
	}
	return nil
}

func setGlobalShortcutsPaused(a *app.App, paused bool) {
	if paused && usesShortcutPortal() && a != nil && (a.HasAnyShortcutPending() || portalTimedOut) {
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
// active slots. When the Wayland GlobalShortcuts portal is active, candidate is
// kept in pendingTargets until OnKeys confirms it so quitting or declining
// preserves the last confirmed binding in settings.json.
func trySetShortcut(a *app.App, slot, candidate string) bool {
	if a == nil {
		return false
	}
	if usesShortcutPortal() && a.HasAnyShortcutPending() {
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

	candSlot := *target
	candSlot.accel = strings.TrimSpace(candidate)
	if portalTimedOut {
		if candSlot.accel == "" {
			clearSlotPendingMeta(slot)
			delete(timedOutCandidates, slot)
			a.SetShortcutErr(slot, "")
			a.SetShortcutBoundKeys(slot, "")
			a.SetShortcutPending(slot, false)
			setSlotAccel(&a.Settings, slot, "")
			a.SaveSettings()
			return true
		}
		delete(timedOutCandidates, slot)
		deferredEdit = &pendingShortcutEdit{slot: slot, candidate: candSlot.accel}
		a.SetShortcutErr(slot, "")
		a.SetShortcutPending(slot, true)
		armPortalTimer(a)
		a.Invalidate()
		return true
	}

	// Ensure all non-target slots are registered first with fresh IDs so
	// duplicate bindings are caught and Wayland portal always delivers OnKeys.
	clearGlobalShortcuts()
	for _, s := range slots {
		if s.id == slot || s.accel == "" {
			continue
		}
		if err := registerSlot(a, s); err != nil {
			a.SetShortcutErr(s.id, err.Error())
		}
	}

	if candSlot.accel == "" {
		clearSlotPendingMeta(slot)
		a.SetShortcutErr(slot, "")
		a.SetShortcutBoundKeys(slot, "")
		a.SetShortcutPending(slot, false)
		setSlotAccel(&a.Settings, slot, "")
		a.SaveSettings()
		return true
	}

	portalMode := usesShortcutPortal()
	prevRollback, hadRollback := pendingRollback[slot], pendingHasRollback[slot]
	clearSlotPendingMeta(slot)
	if portalMode {
		pendingTargets[slot] = candSlot.accel
	} else if runtime.GOOS == "linux" && strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != "" {
		baseline := target.accel
		if hadRollback {
			baseline = prevRollback
		}
		if !strings.EqualFold(baseline, candSlot.accel) {
			pendingRollback[slot] = baseline
			pendingHasRollback[slot] = true
		}
	}

	if err := registerSlot(a, candSlot); err != nil {
		clearSlotPendingMeta(slot)
		if hadRollback && !portalMode {
			pendingRollback[slot] = prevRollback
			pendingHasRollback[slot] = true
		}
		if target.accel != "" {
			_ = registerSlot(a, *target)
		}
		a.SetShortcutErr(slot, err.Error())
		a.Invalidate()
		return false
	}

	if !portalMode {
		setSlotAccel(&a.Settings, slot, candSlot.accel)
		a.SaveSettings()
	} else {
		a.Invalidate()
	}
	return true
}

func applyGlobalShortcuts(a *app.App) {
	if a == nil || shortcutsPaused || portalTimedOut {
		return
	}
	slots := shortcutSlots(a)
	// Preserve any in-flight Wayland portal candidate so saving an unrelated
	// setting (such as Appearance) does not unregister the pending candidate.
	for i := range slots {
		if pending, ok := pendingTargets[slots[i].id]; ok && pending != "" {
			slots[i].accel = pending
		}
	}

	desired := make(map[string]string, len(slots))
	desiredNonEmpty := 0
	for _, s := range slots {
		desired[s.id] = s.accel
		if s.accel != "" {
			desiredNonEmpty++
		}
	}

	willUnregister := false
	for id, prev := range registeredShortcuts {
		if prev == "" || desired[id] != prev {
			willUnregister = true
			break
		}
	}

	// On Wayland, unregistering a slot while others remain triggers an async
	// MyGo portal syncShortcuts() for the remaining slots, but MyGo suppresses
	// OnKeys for unchanged IDs. Resetting all slots gives remaining slots fresh
	// IDs so MyGo delivers OnKeys cleanly when the portal replies (no 30s stall).
	if willUnregister && usesShortcutPortal() && desiredNonEmpty > 0 {
		clearGlobalShortcuts()
	} else {
		for id, prev := range registeredShortcuts {
			if prev == "" || desired[id] != prev {
				if prev != "" {
					mygo.GlobalShortcut.Unregister(prev)
				}
				delete(registeredShortcuts, id)
			}
		}
	}

	for _, s := range slots {
		if s.accel == "" {
			clearSlotPendingMeta(s.id)
			a.SetShortcutErr(s.id, "")
			a.SetShortcutBoundKeys(s.id, "")
			a.SetShortcutPending(s.id, false)
			continue
		}
		if registeredShortcuts[s.id] == s.accel && mygo.GlobalShortcut.IsRegistered(s.accel) {
			continue
		}
		if err := registerSlot(a, s); err != nil {
			clearSlotPendingMeta(s.id)
			a.SetShortcutErr(s.id, err.Error())
		}
	}

	if !a.HasAnyShortcutPending() {
		stopPortalTimer()
	}
}

func clearGlobalShortcuts() {
	stopPortalTimer()
	portalTimedOut = false
	portalDrainScheduled = false
	deferredEdit = nil
	timedOutReplyKeys = map[string]string{}
	timedOutCandidates = map[string]bool{}
	for id, acc := range registeredShortcuts {
		if acc != "" {
			mygo.GlobalShortcut.Unregister(acc)
		}
		delete(registeredShortcuts, id)
	}
	mygo.GlobalShortcut.UnregisterAll()
}
