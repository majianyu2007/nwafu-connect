//go:build darwin

package dockhide

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#import <dispatch/dispatch.h>

static void applyNWAFUConnectAccessoryPolicy(void *context) {
	[[NSApplication sharedApplication] setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

static void hideNWAFUConnectDockIcon(void) {
	dispatch_async_f(dispatch_get_main_queue(), NULL, applyNWAFUConnectAccessoryPolicy);
}
*/
import "C"

func Hide() {
	C.hideNWAFUConnectDockIcon()
}
