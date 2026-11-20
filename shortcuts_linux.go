//go:build linux && !android && (amd64 || arm64)

package main

import (
	"os"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	linuxPortalOnce sync.Once
	linuxPortalOK   bool
)

// hasLinuxShortcutPortal reports whether the current Linux Wayland/XWayland
// desktop exposes org.freedesktop.portal.GlobalShortcuts (version > 0),
// matching MyGo's own backend selection in internal/linux/hotkey.go.
func hasLinuxShortcutPortal() bool {
	if strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) == "" {
		return false
	}
	linuxPortalOnce.Do(func() {
		linuxPortalOK = probeGlobalShortcutsPortal()
	})
	return linuxPortalOK
}

func probeGlobalShortcutsPortal() bool {
	gio, err := purego.Dlopen("libgio-2.0.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	glib, err := purego.Dlopen("libglib-2.0.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	gobj, err := purego.Dlopen("libgobject-2.0.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}

	var (
		gBusGetSync             func(busType int32, cancellable uintptr, err *uintptr) uintptr
		gDBusConnectionCallSync func(conn uintptr, name, path, iface, method *byte, params, replyType uintptr, flags, timeout int32, cancellable uintptr, err *uintptr) uintptr
		gVariantNewString       func(s *byte) uintptr
		gVariantNewTuple        func(children unsafe.Pointer, n uintptr) uintptr
		gVariantGetChildValue   func(v uintptr, i uintptr) uintptr
		gVariantGetVariant      func(v uintptr) uintptr
		gVariantTypeNew         func(s *byte) uintptr
		gVariantTypeFree        func(t uintptr)
		gVariantIsOfType        func(v, t uintptr) int32
		gVariantGetUint32       func(v uintptr) uint32
		gVariantUnref           func(v uintptr)
		gErrorFree              func(e uintptr)
		gObjectUnref            func(obj uintptr)
	)
	bind := func(lib uintptr, dst any, sym string) bool {
		addr, symErr := purego.Dlsym(lib, sym)
		if symErr != nil || addr == 0 {
			return false
		}
		purego.RegisterFunc(dst, addr)
		return true
	}
	if !bind(gio, &gBusGetSync, "g_bus_get_sync") ||
		!bind(gio, &gDBusConnectionCallSync, "g_dbus_connection_call_sync") ||
		!bind(glib, &gVariantNewString, "g_variant_new_string") ||
		!bind(glib, &gVariantNewTuple, "g_variant_new_tuple") ||
		!bind(glib, &gVariantGetChildValue, "g_variant_get_child_value") ||
		!bind(glib, &gVariantGetVariant, "g_variant_get_variant") ||
		!bind(glib, &gVariantTypeNew, "g_variant_type_new") ||
		!bind(glib, &gVariantTypeFree, "g_variant_type_free") ||
		!bind(glib, &gVariantIsOfType, "g_variant_is_of_type") ||
		!bind(glib, &gVariantGetUint32, "g_variant_get_uint32") ||
		!bind(glib, &gVariantUnref, "g_variant_unref") ||
		!bind(glib, &gErrorFree, "g_error_free") ||
		!bind(gobj, &gObjectUnref, "g_object_unref") {
		return false
	}

	const gBusTypeSession = 2
	var gerr uintptr
	conn := gBusGetSync(gBusTypeSession, 0, &gerr)
	if gerr != 0 {
		gErrorFree(gerr)
		gerr = 0
	}
	if conn == 0 {
		return false
	}
	defer gObjectUnref(conn)

	cstr := func(s string) *byte {
		b := make([]byte, len(s)+1)
		copy(b, s)
		return &b[0]
	}

	ifaceArg := gVariantNewString(cstr("org.freedesktop.portal.GlobalShortcuts"))
	propArg := gVariantNewString(cstr("version"))
	children := [2]uintptr{ifaceArg, propArg}
	params := gVariantNewTuple(unsafe.Pointer(&children[0]), 2)

	res := gDBusConnectionCallSync(
		conn,
		cstr("org.freedesktop.portal.Desktop"),
		cstr("/org/freedesktop/portal/desktop"),
		cstr("org.freedesktop.DBus.Properties"),
		cstr("Get"),
		params,
		0,
		0,
		3000,
		0,
		&gerr,
	)
	if gerr != 0 {
		gErrorFree(gerr)
	}
	if res == 0 {
		return false
	}
	defer gVariantUnref(res)

	child := gVariantGetChildValue(res, 0)
	if child == 0 {
		return false
	}
	inner := gVariantGetVariant(child)
	gVariantUnref(child)
	if inner == 0 {
		return false
	}
	defer gVariantUnref(inner)

	ut := gVariantTypeNew(cstr("u"))
	if ut == 0 {
		return false
	}
	defer gVariantTypeFree(ut)
	if gVariantIsOfType(inner, ut) == 0 {
		return false
	}
	return gVariantGetUint32(inner) > 0
}
