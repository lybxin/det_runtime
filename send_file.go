// +build sendfile

package main

import (
	"fmt"
	"runtime_det/common/action"

	"github.com/gofrs/uuid"
)

//"127.0.0.1:14455"

func main() {
	for {

		fmt.Println("请输入一个全路径的文件:")
		//  获取命令行参数
		var path string
		fmt.Scan(&path)

		req := &action.CmdRequest{
			RequestId:  uuid.Must(uuid.NewV4()).String(),
			HttpAction: string(action.FunSndFile),
			Args:       path,
			Pwd:        "",
			Ext:        "127.0.0.1:14455",
		}
		httpActionFun, ok := action.HttpActionTable[action.HttpAction(req.HttpAction)]
		if !ok {
			fmt.Printf("invalid action:%s\n", req.HttpAction)
			return
		}

		resp := httpActionFun(req)
		fmt.Printf("req:%s,\n,resp:%s\n", req, resp)

	}
}
