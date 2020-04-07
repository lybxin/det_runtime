package desp

import (
	"fmt"
	"net"
	//"runtime_det/common"
)

//MakeSimpleStr make a simple string
func MakeSimpleStr(str string) ([]byte, int) {
	return []byte(fmt.Sprintf("+%s\r\n", str)), (len(str) + 3)
}

func SendSimpleStr(str string, conn net.Conn) (int, error) {
	simpleStr, num := MakeSimpleStr(str)
	n, err := conn.Write(simpleStr)
	if err != nil {
		return n, fmt.Errorf("write str:%v failed:%v", string(simpleStr), err.Error())
	}
	if num != n {
		return n, fmt.Errorf("write str:%v failed, unmatched bytes:[%d, %d],", string(simpleStr), n, num)
	}
	return n, nil
}
