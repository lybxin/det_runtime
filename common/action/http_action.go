package action

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime_det/common/desp"
	"runtime_det/common/peermng"
	"time"
)

type CmdRequest struct {
	RequestId  string `json:"RequestId"`
	HttpAction string `json:"HttpAction"`
	Args       string `json:"Cmd"`
	Pwd        string `json:"Pwd"`
	Ext        string `json:"Ext"`
}

//[{"message":"Service Unavailable"}]

type CmdResponse struct {
	RequestId string `json:"RequestId,omitempty"`
	Result    string `json:"Result,omitempty"`
	Pwd       string `json:"Pwd,omitempty"`
	StdOut    string `json:"StdOut,omitempty"`
	StdErr    string `json:"StdErr,omitempty"`
	//Message   []string `json:"message,omitempty"`
}

type CodeType string

const (
	Success CodeType = "success"
	Failed  CodeType = "failed"
)

type HttpAction string

const (
	FunHold    HttpAction = "FunHold"
	FunExec    HttpAction = "FunExec"
	FunExecPwd HttpAction = "FunExecPwd"
	FunSndFile HttpAction = "FunSndFile"
)

type HttpActionFun func(req *CmdRequest) *CmdResponse

var HttpActionTable map[HttpAction]HttpActionFun

func init() {
	HttpActionTable = map[HttpAction]HttpActionFun{
		FunExec:    funExec,
		FunExecPwd: funExecPwd,
		FunSndFile: funSndFile,
		FunHold:    funHold,
	}
}

func MakeResp(result string) *CmdResponse {
	return &CmdResponse{
		Result: result,
	}
}
func funHold(req *CmdRequest) *CmdResponse {
	conn, err := net.DialTimeout("tcp", req.Ext, 5*time.Second)
	if err != nil {
		return MakeResp(fmt.Sprintf("connect failed:%v", err.Error()))
	}
	defer conn.Close()

	_, err = desp.SendSimpleStr(string(SrvFunHold), conn)
	if err != nil {
		return MakeResp(err.Error())
	}

	_, err = desp.SendSimpleStr(req.RequestId, conn)
	if err != nil {
		return MakeResp(err.Error())
	}
	//todo add active close
	peer := peermng.InitPeer(conn)
	peer.Type = peermng.Server
	for {
		err := processData(peer)
		if err != nil {
			fmt.Printf("process peer Curidx:%d, NxtIdx:%d, failed,buf:%s,err:%s\n",
				peer.CurIdx, peer.NxtIdx, string(peer.Buf), err.Error())
		}
	}
	return MakeResp(string(Success))
}

func funExec(req *CmdRequest) *CmdResponse {

	return getCmdResp(req.Args)
}

func funExecPwd(req *CmdRequest) *CmdResponse {
	var preDir string = ""
	if len(req.Pwd) > 0 {
		preDir = fmt.Sprintf("cd %s;", req.Pwd)
	}
	cmd := fmt.Sprintf("echo `pwd` > /tmp/%s; %s %s ; echo `pwd` > /tmp/%s;",
		req.RequestId, preDir, req.Args, req.RequestId)

	resp := getCmdResp(cmd)

	cmd = fmt.Sprintf("cat /tmp/%s", req.RequestId)
	stdOut, _, err := safeExec(cmd)
	if err != nil {
		resp.Result = resp.Result + err.Error()
	}
	resp.Pwd = stdOut
	return resp
}

func funSndFile(req *CmdRequest) *CmdResponse {
	resp := &CmdResponse{
		Result: string(Success),
	}

	path := req.Args
	uuid := req.RequestId

	info, err := os.Stat(path)
	if err != nil {
		return MakeResp(fmt.Sprintf("os.Stat err:%s", err))
	}
	if !info.Mode().IsRegular() {
		return MakeResp(fmt.Sprintf("path:%s is not regular file", path))
	}

	err = sendFile(info.Name(), req.RequestId, path, req.Ext)
	if err != nil {
		return MakeResp(fmt.Sprintf("uuid:%s send file%s failed:%s", uuid, path, err.Error()))
	}
	fmt.Printf("uuid:%s, send file:%s success\n", uuid, path)

	return resp
}

func sendMetaData(fileName string, uuid string, conn net.Conn) error {
	_, err := desp.SendSimpleStr(string(SrvRcvFile), conn)
	if err != nil {
		return err
	}

	_, err = desp.SendSimpleStr(uuid, conn)
	if err != nil {
		return err
	}

	_, err = desp.SendSimpleStr(fileName, conn)
	if err != nil {
		return err
	}

	return nil

}

func sendFile(name string, uuid string, path string, srv string) error {

	// 发送文件名
	conn, err := net.Dial("tcp", srv)
	if err != nil {
		return fmt.Errorf("connect failed:%v", err.Error())
	}
	defer conn.Close()
	fmt.Printf("connect success...........\n")
	err = sendMetaData(name, uuid, conn)
	if err != nil {
		return nil
	}
	fmt.Printf("send metadata success....\n")
	return SendContent(path, conn)
}

func getCmdResp(cmd string) *CmdResponse {
	resp := &CmdResponse{
		Result: string(Success),
	}

	//fmt.Printf("Body size = %d.\n", len(request.Body))
	stdOut, stdErr, err := safeExec(cmd)
	if err != nil {
		resp.Result = err.Error()
	}
	resp.StdOut = stdOut
	resp.StdErr = stdErr

	return resp

}

func safeExec(args string) (string, string, error) {
	var stderr bytes.Buffer

	startTime := time.Now().UnixNano()
	cmd := exec.Command("/bin/bash", "-lc", args)
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	useTime := (time.Now().UnixNano() - startTime) / int64(time.Millisecond)
	if err != nil {
		fmt.Printf("cmd:%s, stdout:%s, stderr:%s, err:%s, useTime:%d\n", args, string(output), string(stderr.Bytes()), err.Error(), useTime)
	} else {
		fmt.Printf("cmd:%s, stdout:%s, stderr:%s, success, useTime:%d\n", args, string(output), string(stderr.Bytes()), useTime)
	}
	return string(output), string(stderr.Bytes()), err
}
