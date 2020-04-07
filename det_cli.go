// +build cli

package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"runtime_det/common/action"
	"runtime_det/common/desp"
	"runtime_det/common/peermng"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/peterh/liner"
)

type DetSet string

const (
	DetBaseCmd DetSet = "det"
	SetUrl     DetSet = "url"
	SetDebug   DetSet = "debug"
	SetSrv     DetSet = "server"
	SetFollow  DetSet = "pwdfollow"
	Download   DetSet = "dl"
	HoldOn     DetSet = "hold"
)

var holdPeer *peermng.Peer

var (
	funUrl  string = ""
	srvAddr string = ""
	debug   bool   = false
	follow  bool   = true

	timeOut string = "503 Service Unavailable"
)

type setFun func([]string) error

var detSetTable map[DetSet]setFun = map[DetSet]setFun{
	SetUrl:    setUrl,
	SetDebug:  setDebug,
	SetFollow: setFollow,
	Download:  download,
	SetSrv:    setSrv,
	HoldOn:    holdOn,
}

func sendHoldOnToFun(reqStr string) error {
	// 3.remote exec
	resp, err := getResp(reqStr)
	if err != nil {
		if err.Error() == timeOut {
			fmt.Printf("%s,apigw may be times out in holdon mode.\n", err.Error())
			return nil
		}
		return err
	}
	return fmt.Errorf("unexpected result: we should timeout%s", resp.Result)
}

// todo add command format prompt
func holdOn(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("args num is error\n")
	}

	if args[2] == "off" {
		if holdPeer == nil {
			return nil
		}
		holdPeer.Conn.Close()
		holdPeer = nil
		return nil
	}
	if args[2] != "on" {
		return fmt.Errorf("parameter error, wo only receive \"on\" and \"off\"\n")
	}
	if holdPeer != nil {
		return fmt.Errorf("we are already holdon\n")
	}

	id, reqStr, err := constructReq(action.FunHold, "", srvAddr)
	if err != nil {

		return fmt.Errorf("%s\n", err.Error())
	}

	go sendHoldOnToFun(reqStr)
	// if err != nil{
	// 	return err
	// }
	conn, err := net.DialTimeout("tcp", srvAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect failed:%v", err.Error())
	}
	holdPeer = peermng.InitPeer(conn)
	holdPeer.RequestId = id
	err = action.HoldOnServer(holdPeer)
	if err != nil {
		holdPeer.Conn.Close()
		holdPeer = nil
		return err
	}

	return nil
}

// todo add command format prompt
func setUrl(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("args num is error，must specify a url\n")
	}
	funUrl = args[2]
	return nil
}

// todo add command format prompt
func setSrv(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("args num is error，must specify a url\n")
	}
	srvAddr = args[2]
	return nil
}

func setDebug(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("args num is error\n")
	}
	if args[2] == "off" {
		debug = false
		return nil
	}
	if args[2] == "on" {
		debug = true
		return nil
	}
	return fmt.Errorf("parameter error, wo only receive \"on\" and \"off\"\n")
}
func setFollow(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("args num is error \n")
	}
	if args[2] == "off" {
		follow = false
		pwd = ""
		return nil
	}
	if args[2] == "on" {
		follow = true
		return nil
	}
	return fmt.Errorf("parameter error, we only receive \"on\" and \"off\"\n")
}

func download(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("args num is error:det dl remote_path\n")
	}
	var path string
	if filepath.IsAbs(args[2]) {
		path = args[2]
	} else {
		path = filepath.Clean(filepath.Join(pwd, args[2]))
	}
	id, reqStr, err := constructReq(action.FunSndFile, path, srvAddr)
	if err != nil {

		return fmt.Errorf("%s\n", err.Error())
	}

	// 3.remote exec
	resp, err := getResp(reqStr)
	if err != nil {
		if err.Error() == timeOut {
			fmt.Printf("%s,apigw may be times out. if you are downloading a file,\n"+
				"we will continue to download in backgroud until the function times out.\n", err.Error())
			return getFile(id, filepath.Base(path))
		} else {
			return fmt.Errorf("getResp failed:%s", err.Error())
		}
	}
	if resp.Result != string(action.Success) {
		return fmt.Errorf("err result:%s", resp.Result)
	}

	return getFile(id, filepath.Base(path))

}

// todo timeout
func getFile(id string, name string) error {

	conn, err := net.DialTimeout("tcp", srvAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect failed:%v", err.Error())
	}
	//defer conn.Close()
	_, err = desp.SendSimpleStr(string(action.SrvSndFile), conn)
	if err != nil {
		return err
	}

	_, err = desp.SendSimpleStr(id, conn)
	if err != nil {
		return err
	}

	_, err = desp.SendSimpleStr(name, conn)
	if err != nil {
		return err
	}
	go action.ProcessConn(conn)
	return nil
}

type Client struct {
	client *http.Client
}

func NewClient(timeOut time.Duration) *Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 20 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          500,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 50 * time.Second,
		ExpectContinueTimeout: 50 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	return &Client{
		client: &http.Client{
			Timeout:   timeOut,
			Transport: tr,
		},
	}
}

var DefaultClient = NewClient(300 * time.Second)

func (c *Client) sendRequest(method, url string, reqBody []byte) ([]byte, error) {
	startTime := time.Now()

	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Println(err)
		}
	}()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if debug {
		fmt.Printf("send request [%+v] to [%s], resp:%+v, response body: [%s], costTime: %s\n",
			string(reqBody), url, resp, string(body), time.Since(startTime))
	}

	if resp.StatusCode == 503 {
		return nil, fmt.Errorf(timeOut)
		//fmt.Errorf("%s,apigw may be times out. if you are downloading a file,\n“+
		//”we will continue to download in backgroud until the function times out.\n"+
		//"if you want to perform time-consuming operations, consider switching to \n" +
		//"\"holdon\" mode", resp.Status)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http response status exception, status: %s", resp.Status)
	}

	if body == nil {
		return nil, fmt.Errorf("http response body is nil, status: %s, costTime:%s",
			resp.Status, time.Since(startTime))
	}

	return body, nil
}

var pwd string = "[invalid]"

func constructReq(httpAction action.HttpAction, args string, ext string) (string, string, error) {
	cmdReq := action.CmdRequest{
		RequestId:  uuid.Must(uuid.NewV4()).String(),
		HttpAction: string(httpAction),
		Args:       args,
		Pwd:        pwd,
		Ext:        ext,
	}
	dataStr, err := json.Marshal(cmdReq)
	if err != nil {
		return "", "", err
	}
	return cmdReq.RequestId, string(dataStr), nil
}

func getResp(dataStr string) (*action.CmdResponse, error) {
	var cmdRsp []byte
	var err error
	if holdPeer == nil {
		cmdRsp, err = DefaultClient.sendRequest("POST", funUrl, []byte(dataStr))
		if err != nil {
			return nil, err
		}

	} else {
		var tcpRsp string
		tcpRsp, err = action.ProxyCmd(holdPeer, dataStr)
		if err != nil {
			holdPeer.Conn.Close()
			holdPeer = nil
			return nil, err
		}
		cmdRsp = []byte(tcpRsp)
	}

	resp := &action.CmdResponse{}
	if err := json.Unmarshal(cmdRsp, resp); err != nil {
		return nil, fmt.Errorf("Unmarshal resp:[%s] internal error occurred:%s", string(cmdRsp),err.Error())
	}
	if resp.Result != string(action.Success) {
		fmt.Println(resp.Result)
	}
	resp.StdErr = strings.Replace(resp.StdErr, "id: cannot find name for group ID 495\n", "", 1)
	return resp, nil

}

func processLine(text string) {
	text = strings.TrimRight(strings.TrimSpace(text), ";")
	text = strings.Replace(text, "\r\n", "\n", -1)
	if len(text) == 0 {
		return
	}
	// 1.process inner command
	args := strings.Split(text, " ")
	if args[0] == string(DetBaseCmd) {
		if detSetFun, ok := detSetTable[DetSet(args[1])]; ok {
			err := detSetFun(args)
			if err == nil {
				fmt.Printf("\033[1;32;40msuccess\033[0m\n")
			} else {
				fmt.Printf("\033[1;31;40m%s exec failed:%s\033[0m\n", args[1], err.Error())
			}
			return
		} else {
			fmt.Printf("\033[1;31;40mdet inner commond not found:%s\033[0m\n", args[1])
			return
		}
	}
	// 2.construct remote req
	var httpAction action.HttpAction
	if follow {
		httpAction = action.FunExecPwd
	} else {
		httpAction = action.FunExec
	}
	_, reqStr, err := constructReq(httpAction, text, "")
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	// 3.remote exec
	resp, err := getResp(reqStr)
	if err != nil {
		if err.Error() == timeOut {
			fmt.Printf("%s,apigw may be times out. if you want to perform time-consuming operations,\n"+
				" consider switching to \"holdon\" mode\n", err.Error())
		} else {
			fmt.Printf("getResp failed:%s\n", err.Error())
		}

		return
	}
	// 4. local display
	//fmt.Printf("%v", resp.Message)
	fmt.Printf(resp.StdErr)
	fmt.Printf(resp.StdOut)
	if len(resp.Pwd) > 0 {
		pwd = resp.Pwd
	}
	return
}

func main() {

	lineInput := liner.NewLiner()
	defer lineInput.Close()
	fmt.Println("Simple Remote Shell")
	fmt.Printf("---------------------\n")
	lineInput.SetCtrlCAborts(true)
	for {
		pwd = strings.Replace(pwd, "\n", "", -1)

		if text, err := lineInput.Prompt(fmt.Sprintf("%s-> ", pwd)); err == nil {
			processLine(text)
			lineInput.AppendHistory(text)
		} else if err == liner.ErrPromptAborted {
			fmt.Print("Aborted")
			break
		} else {
			fmt.Print("Error reading line: ", err)
			break
		}

	}

}
