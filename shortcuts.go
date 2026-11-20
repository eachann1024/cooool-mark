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
	registeredShortcuts       = map[string]string{}
	pendingTargets            = map[string]string{}
	lateConfirmTargets        = map[string]string{}
	pendingRollback           = map[string]string{}
	pendingHasRollback        = map[string]bool{}
	xwaylandSuperseded        = map[string]bool{}
	xwaylandCrossSlot         = map[string]bool{}
	xwaylandSlotOfAccel       = map[string]string{}
	pendingSeqs               = map[string]uint64{}
	portalBatchReplies        = map[string]string{}
	timedOutCandidates        = map[string]bool{}
	deferredEdit              *pendingShortcutEdit
	portalTimer               *time.Timer
	portalTimerSeq            uint64
	portalBusy                bool
	portalBatchScheduled      bool
	portalTimedOut            bool
	portalNeedsResync         bool
	portalNeedsResyncOnResume bool
	portalObserved            bool
	xwaylandSentUnconfirmed   bool
	shortcutsPaused           bool
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
				if shortcutsPaused {
					return
				}
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
				if shortcutsPaused {
					return
				}
				a.QuickAddFromClipboard()
			},
		},
		{
			id:          "notch",
			accel:       strings.TrimSpace(a.Settings.ShortcutNotch),
			description: locales.SettingsShortcutNotch(),
			run: func() {
				if shortcutsPaused {
					return
				}
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
				if shortcutsPaused {
					return
				}
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
	delete(lateConfirmTargets, slotID)
	delete(pendingRollback, slotID)
	delete(pendingHasRollback, slotID)
	delete(xwaylandSuperseded, slotID)
	delete(xwaylandCrossSlot, slotID)
}

// cancelPendingShortcut drops any queued or in-flight candidate edit for slot
// (for example when the user turns Global Search off while a search shortcut
// change is awaiting Wayland portal confirmation).
func cancelPendingShortcut(a *app.App, slot string) {
	if deferredEdit != nil && deferredEdit.slot == slot {
		deferredEdit = nil
	}
	delete(timedOutCandidates, slot)
	clearSlotPendingMeta(slot)
	if portalBusy {
		portalNeedsResync = true
	}
	if a != nil {
		a.SetShortcutPending(slot, false)
		a.SetShortcutBoundKeys(slot, "")
		a.SetShortcutErr(slot, "")
		if !a.HasAnyShortcutPending() {
			stopPortalTimer()
		}
	}
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

// finishPortalBatch runs on the next main-loop tick after MyGo's synchronous
// reportShortcuts pass completes (so MyGo's internal p.busy is false). Because
// we never mutate MyGo shortcuts while portalBusy is true, p.stale is also
// false here, letting us cleanly apply replies, run any queued edit, or resync
// the OS back to a.Settings without overlapping portal sessions.
func finishPortalBatch(a *app.App) {
	if !portalBatchScheduled {
		return
	}
	portalBatchScheduled = false
	portalBusy = false
	stopPortalTimer()

	replies := portalBatchReplies
	portalBatchReplies = map[string]string{}
	wasTimedOut := portalTimedOut
	portalTimedOut = false
	prevCandidates := timedOutCandidates
	timedOutCandidates = map[string]bool{}
	edit := deferredEdit
	deferredEdit = nil
	needsResync := portalNeedsResync
	portalNeedsResync = false

	for id := range a.ShortcutPending {
		a.SetShortcutPending(id, false)
	}

	rejectedSlots := map[string]bool{}
	settingsDirty := false

	if !wasTimedOut {
		for id, keys := range replies {
			acc := registeredShortcuts[id]
			trimmedKeys := strings.TrimSpace(keys)

			pendingAcc, hasPendingTarget := pendingTargets[id]
			matchesPendingTarget := hasPendingTarget && pendingAcc == acc
			lateAcc, hasLateConfirm := lateConfirmTargets[id]
			matchesLateConfirm := hasLateConfirm && lateAcc == acc
			hasRollback := pendingHasRollback[id]
			rollbackAcc := pendingRollback[id]

			if matchesPendingTarget || hasRollback || matchesLateConfirm {
				delete(pendingTargets, id)
				delete(lateConfirmTargets, id)
				delete(pendingRollback, id)
				delete(pendingHasRollback, id)
				delete(xwaylandSuperseded, id)
				delete(xwaylandCrossSlot, id)
			}

			if trimmedKeys == "" {
				a.SetShortcutBoundKeys(id, "")
				rejectedSlots[id] = true
				a.SetShortcutErr(id, locales.SettingsShortcutRejected())
				if matchesPendingTarget {
					needsResync = true
				} else if hasRollback {
					setSlotAccel(&a.Settings, id, rollbackAcc)
					settingsDirty = true
					needsResync = true
				}
			} else {
				a.SetShortcutBoundKeys(id, keys)
				a.SetShortcutErr(id, "")
				if matchesPendingTarget {
					setSlotAccel(&a.Settings, id, pendingAcc)
					settingsDirty = true
				} else if matchesLateConfirm {
					setSlotAccel(&a.Settings, id, lateAcc)
					settingsDirty = true
				}
			}
		}
	}

	if settingsDirty {
		prevPaused := shortcutsPaused
		shortcutsPaused = true
		a.SaveSettings()
		shortcutsPaused = prevPaused
	}

	if edit != nil {
		trySetShortcut(a, edit.slot, edit.candidate)
		return
	}

	if wasTimedOut {
		candidateRejected := false
		for id := range prevCandidates {
			if strings.TrimSpace(replies[id]) == "" {
				candidateRejected = true
				break
			}
		}
		if !candidateRejected && !needsResync && shortcutsMatchSettings(a) {
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
		if shortcutsPaused {
			portalNeedsResyncOnResume = true
			a.Invalidate()
			return
		}
		clearGlobalShortcuts()
		applyGlobalShortcuts(a)
		for id := range prevCandidates {
			if strings.TrimSpace(replies[id]) == "" || slotAccel(a, id) == "" {
				a.SetShortcutErr(id, locales.SettingsShortcutRejected())
			}
		}
		a.Invalidate()
		return
	}

	if shortcutsPaused {
		if needsResync || (len(lateConfirmTargets) == 0 && !shortcutsMatchSettings(a)) {
			portalNeedsResyncOnResume = true
		}
		a.Invalidate()
		return
	}

	if needsResync || (len(lateConfirmTargets) == 0 && !shortcutsMatchSettings(a)) {
		clearGlobalShortcuts()
		applyGlobalShortcuts(a)
		for id := range rejectedSlots {
			a.SetShortcutErr(id, locales.SettingsShortcutRejected())
		}
		a.Invalidate()
		return
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
				portalBusy = true
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
	wrappedRun := func() {
		if shortcutsPaused {
			return
		}
		cur := slotAccel(a, slotID)
		if cur == "" || !strings.EqualFold(cur, acc) {
			return
		}
		if fn != nil {
			fn()
		}
	}
	pendingSeqs[slotID]++
	seq := pendingSeqs[slotID]
	a.SetShortcutBoundKeys(slotID, "")
	a.SetShortcutPending(slotID, false)

	err := mygo.GlobalShortcut.RegisterWith(acc, mygo.GlobalShortcutOptions{
		Description: s.description,
		OnKeys: func(keys string) {
			portalObserved = true
			xwaylandSentUnconfirmed = false
			if registeredShortcuts[slotID] != acc || pendingSeqs[slotID] != seq {
				return
			}
			trimmedKeys := strings.TrimSpace(keys)

			// XWayland probe-disagreement edge cases (possible only when an
			// unconfirmed XWayland bind was superseded before the portal was
			// detected): handle without mutating MyGo so MyGo's own
			// `if p.stale { b.syncShortcuts() }` presents any newer candidate
			// without triggering a duplicate dialog if this was already the
			// final rejection.
			if trimmedKeys == "" && xwaylandSuperseded[slotID] {
				delete(xwaylandSuperseded, slotID)
				delete(xwaylandCrossSlot, slotID)
				hasRollback := pendingHasRollback[slotID]
				rollbackAcc := pendingRollback[slotID]
				delete(pendingRollback, slotID)
				delete(pendingHasRollback, slotID)
				a.SetShortcutBoundKeys(slotID, "")
				if hasRollback {
					setSlotAccel(&a.Settings, slotID, rollbackAcc)
					prevPaused := shortcutsPaused
					shortcutsPaused = true
					a.SaveSettings()
					shortcutsPaused = prevPaused
				}
				lateConfirmTargets[slotID] = acc
				a.SetShortcutErr(slotID, locales.SettingsShortcutRejected())
				a.Invalidate()
				return
			}
			if trimmedKeys != "" && xwaylandCrossSlot[slotID] {
				// This non-empty reply may come from another slot's earlier
				// BindShortcuts for the same accelerator. Keep pendingRollback
				// intact so a subsequent rejection of this slot still restores
				// its confirmed baseline in settings.json.
				delete(xwaylandCrossSlot, slotID)
				delete(xwaylandSuperseded, slotID)
				a.SetShortcutBoundKeys(slotID, keys)
				a.SetShortcutErr(slotID, "")
				a.Invalidate()
				return
			}

			portalBatchReplies[slotID] = keys
			if !portalBatchScheduled {
				portalBatchScheduled = true
				a.Post(func() {
					finishPortalBatch(a)
				})
			}
		},
	}, wrappedRun)
	if err != nil {
		return err
	}
	registeredShortcuts[slotID] = acc
	a.SetShortcutErr(slotID, "")
	if usesShortcutPortal() && a.ShortcutBoundKeys(slotID) == "" {
		portalBusy = true
		a.SetShortcutPending(slotID, true)
		armPortalTimer(a)
	} else if runtime.GOOS == "linux" && strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != "" && !usesShortcutPortal() {
		xwaylandSentUnconfirmed = true
		xwaylandSlotOfAccel[strings.ToLower(acc)] = slotID
	}
	return nil
}

func setGlobalShortcutsPaused(a *app.App, paused bool) {
	if paused && usesShortcutPortal() && a != nil && a.HasAnyShortcutPending() {
		return
	}
	if shortcutsPaused == paused {
		return
	}
	shortcutsPaused = paused
	if paused {
		if usesShortcutPortal() {
			return
		}
		clearGlobalShortcuts()
		return
	}
	if portalNeedsResyncOnResume {
		portalNeedsResyncOnResume = false
		if !portalBusy {
			clearGlobalShortcuts()
		}
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
	portalNeedsResyncOnResume = false

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
	if portalBusy {
		if candSlot.accel == "" {
			clearSlotPendingMeta(slot)
			delete(timedOutCandidates, slot)
			portalNeedsResync = true
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

	hadPriorUnconfirmed := xwaylandSentUnconfirmed

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
	prevSuperseded := xwaylandSuperseded[slot]
	prevCrossSlot := xwaylandCrossSlot[slot]
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
		if hadPriorUnconfirmed {
			xwaylandSuperseded[slot] = true
			if prevOwner := xwaylandSlotOfAccel[strings.ToLower(candSlot.accel)]; prevOwner != "" && prevOwner != slot {
				xwaylandCrossSlot[slot] = true
			}
		}
	}

	if err := registerSlot(a, candSlot); err != nil {
		clearSlotPendingMeta(slot)
		if !portalMode {
			if hadRollback {
				pendingRollback[slot] = prevRollback
				pendingHasRollback[slot] = true
			}
			if prevSuperseded {
				xwaylandSuperseded[slot] = true
			}
			if prevCrossSlot {
				xwaylandCrossSlot[slot] = true
			}
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
	if a == nil || shortcutsPaused {
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

	if portalBusy {
		if len(registeredShortcuts) != desiredNonEmpty {
			portalNeedsResync = true
			return
		}
		for _, s := range slots {
			if s.accel == "" {
				if _, ok := registeredShortcuts[s.id]; ok {
					portalNeedsResync = true
					return
				}
				continue
			}
			if registeredShortcuts[s.id] != s.accel {
				portalNeedsResync = true
				return
			}
		}
		return
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
	portalBusy = false
	portalBatchScheduled = false
	portalTimedOut = false
	portalNeedsResync = false
	portalNeedsResyncOnResume = false
	deferredEdit = nil
	portalBatchReplies = map[string]string{}
	timedOutCandidates = map[string]bool{}
	lateConfirmTargets = map[string]string{}
	for id, acc := range registeredShortcuts {
		if acc != "" {
			mygo.GlobalShortcut.Unregister(acc)
		}
		delete(registeredShortcuts, id)
	}
	mygo.GlobalShortcut.UnregisterAll()
}
