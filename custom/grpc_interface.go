package main

import "C"
import v2 "github.com/Darkmen203/rostovvpn-core/v2"

//export StartCoreGrpcServer
func StartCoreGrpcServer(listenAddress *C.char) (CErr *C.char) {
	_, err := v2.StartCoreGrpcServer(C.GoString(listenAddress))
	return emptyOrErrorC(err)
}
