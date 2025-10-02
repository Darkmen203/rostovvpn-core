//go:build bydll

package main

/*
#include <stdlib.h>
#include <stdint.h>

// Import the function from the DLL
char* parseCli(int argc, char** argv);
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/Darkmen203/rostovvpn-core/cli/cmdroot"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tunnel" {
		code := cmdroot.Run(os.Args[1:])
		os.Exit(code)
	}

	args := os.Args

	// Convert []string to []*C.char
	var cArgs []*C.char
	for _, arg := range args {
		cArgs = append(cArgs, C.CString(arg))
	}
	defer func() {
		for _, arg := range cArgs {
			C.free(unsafe.Pointer(arg))
		}
	}()

	// Call the C function
	result := C.parseCli(C.int(len(cArgs)), (**C.char)(unsafe.Pointer(&cArgs[0])))
	fmt.Println(C.GoString(result))
}
