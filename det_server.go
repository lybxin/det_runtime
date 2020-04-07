// +build server

package main

import (
	"fmt"
	"net"
	"runtime_det/common/action"
)

func main() {
	// 创建一个服务器
	Server, err := net.Listen("tcp", "0.0.0.0:14455")
	if err != nil {
		fmt.Println("net.Listen err =", err)
		return
	}
	defer Server.Close()
	// 接受文件名
	for {
		conn, err := Server.Accept()
		if err != nil {
			fmt.Printf("Server.Accept err:%s\n", err.Error())
			return
		}
		fmt.Printf("\n--------accept new conn----------\n")
		go action.ProcessConn(conn)
		action.CleanHoldInfo()
	}
}
