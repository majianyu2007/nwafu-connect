// Command libnwafuconnect builds NWAFU Connect as a C library
// (-buildmode=c-shared or c-archive) for embedding in host apps.
//
// Every function returns a heap-allocated UTF-8 JSON string that the caller
// must release with NcFree:
//
//	{"ok":bool,"state":"idle|starting|awaiting|running|failed|stopped",
//	 "proxy":"127.0.0.1:port","clientIp":"...","error":"...",
//	 "needs":null|"captcha"|"sms"|"totp","image":"<base64>","hint":"..."}
//
// NcStart and NcSubmit block until the session is running, has failed, or
// needs input; call them off the UI thread. When "needs" is set, answer with
// NcSubmit(needs, value). For "captcha", value is the clicked points in image
// pixels, e.g. "[[12,30],[80,41]]". One session per process.
package main

/*
#include <stdlib.h>
*/
import "C"

import "unsafe"

var lib = newBridge(nil)

//export NcStart
func NcStart(configJSON *C.char) *C.char {
	return C.CString(lib.Start(C.GoString(configJSON)))
}

//export NcSubmit
func NcSubmit(kind, value *C.char) *C.char {
	return C.CString(lib.Submit(C.GoString(kind), C.GoString(value)))
}

//export NcStatus
func NcStatus() *C.char {
	return C.CString(lib.Status())
}

//export NcStop
func NcStop() *C.char {
	return C.CString(lib.Stop())
}

//export NcFree
func NcFree(p *C.char) {
	C.free(unsafe.Pointer(p))
}

func main() {}
