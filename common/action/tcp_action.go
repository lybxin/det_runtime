package action

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"runtime_det/common/desp"
	"runtime_det/common/peermng"
	"sync"
	"time"
)

//todo time clean
type HoldPair struct {
	uuid       string
	FunPeer    *peermng.Peer
	TermPeer   *peermng.Peer
	CreateTime time.Time
}

var HoldInfoLock sync.RWMutex
var HoldInfo map[string]*HoldPair = make(map[string]*HoldPair)

type TcpAction string

//todo consider add a rsp action used by all component as a action rsp
const (
	SrvRcvFile   TcpAction = "SrvRcvFile"
	SrvSndFile   TcpAction = "SrvSndFile"
	SrvFunHold   TcpAction = "SrvFunHold"
	SrvTermHold  TcpAction = "SrvProxy"
	SrvFunCmdRsp TcpAction = "SrvFunCmdRsp"
	SrvTermCmd   TcpAction = "srvTermCmd"

	FunCmdReq TcpAction = "FunCmd"

	CliRcvFile TcpAction = "CliRcvFile"

	ActionResult = "ActionResult"
)

//hold on:
//1、term send FunHold to function，function dont reply term
//2、term send SrvTermHold to server
//3、function send SrvFunHold to server [consider reply ActionResult]
//4、server send ActionResult to term
//5、term send SrvTermCmd to server
//6、server send FunCmdReq to function
//7、fun reply server with SrvFunCmdRsp
//8、server reply term with ActionResult
//9、loop step 5

//used by server to mark file-request
var uuidSet map[string]bool = make(map[string]bool)

type TcpActionFun func(peer *peermng.Peer) error

var TcpActionTable map[TcpAction]TcpActionFun

func init() {
	TcpActionTable = map[TcpAction]TcpActionFun{
		SrvRcvFile:   srvRcvFile,
		SrvSndFile:   srvSndFile,
		CliRcvFile:   cliRcvFile,
		FunCmdReq:    funCmdReq,
		SrvFunCmdRsp: srvFunCmdRsp,
		SrvFunHold:   srvFunHold,
		SrvTermHold:  srvTermHold,
		//ActionResult: actionResult,
		SrvTermCmd: srvTermCmd,
	}
}

const (
	ServerFailed = "proxy server failed"
)

func ProcessConn(conn net.Conn) {
	peer := peermng.InitPeer(conn)

	err := processData(peer)
	//fmt.Printf("process peer Curidx:%d, NxtIdx:%d, failed,buf:%s\n",
	//	peer.CurIdx, peer.NxtIdx, string(peer.Buf))
	if err != nil {
		fmt.Printf("process peer Curidx:%d, NxtIdx:%d, failed,buf:%s,err:%s\n",
			peer.CurIdx, peer.NxtIdx, string(peer.Buf), err.Error())
	}
	if peer.TcpAction != string(SrvFunHold) {
		conn.Close()
	}
}

//cmd proxy
func getHoldOnRsp(jsonReq string) *CmdResponse {
	req := &CmdRequest{}
	if err := json.Unmarshal([]byte(jsonReq), req); err != nil {
		return MakeResp("para is invalid json format")
	}
	httpActionFun, ok := HttpActionTable[HttpAction(req.HttpAction)]
	if !ok {
		return MakeResp("invalid action")
	}

	return httpActionFun(req)
}

func makeActionResult(peer *peermng.Peer, uuid string, result string) error {
	_, err := desp.SendSimpleStr(string(ActionResult), peer.Conn)
	if err != nil {
		return err
	}

	_, err = desp.SendSimpleStr(uuid, peer.Conn)
	if err != nil {
		return err
	}

	_, err = desp.SendSimpleStr(result, peer.Conn)
	if err != nil {
		return err
	}
	return nil

}
func waitTermCmd(peer *peermng.Peer) error {
	for {
		err := processData(peer)
		if err != nil {
			return err
		}
	}
}

func CleanHoldInfo() {
	HoldInfoLock.Lock()
	for k, v := range HoldInfo {
		if time.Now().Sub(v.CreateTime) > 2000*time.Second {
			if v.FunPeer != nil {
				v.FunPeer.Conn.Close()
			}
			if v.TermPeer != nil {
				v.TermPeer.Conn.Close()
			}
			delete(HoldInfo, k)
		}

	}
	HoldInfoLock.Unlock()

}

func srvTermCmd(peer *peermng.Peer) error {
	//format:+action\r\n+uuid\r\n+jsonReq\r\n
	uuid, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get uuid failed%s", err.Error())
	}
	jsonReq, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get jsonreq failed%s", err.Error())
	}

	HoldInfoLock.RLock()
	pairInfo, ok := HoldInfo[uuid]
	if !ok {
		resp := &CmdResponse{
			Result: ServerFailed,
		}
		failRsp, _ := json.Marshal(resp)
		makeActionResult(peer, uuid, string(failRsp))
	}
	funPeer := pairInfo.FunPeer
	HoldInfoLock.RUnlock()

	_, err = desp.SendSimpleStr(string(FunCmdReq), funPeer.Conn)
	if err != nil {
		return fmt.Errorf("send FunCmdReq action failed%s", err.Error())
	}

	_, err = desp.SendSimpleStr(jsonReq, funPeer.Conn)
	if err != nil {
		return fmt.Errorf("send jsonReq failed%s", err.Error())
	}
	return processData(funPeer)

}
func cmdProxyLoop(peer *peermng.Peer, uuid string) error {

	err := makeActionResult(peer, uuid, string(Success))
	if err != nil {
		return err
	}
	for {
		err := processData(peer)
		if err != nil {
			fmt.Printf("process peer Curidx:%d, NxtIdx:%d, failed,buf:%s,err:%s\n",
				peer.CurIdx, peer.NxtIdx, string(peer.Buf), err.Error())
			return err
		}
	}
}

func srvTermHold(peer *peermng.Peer) error {
	//format:+action\r\n+uuid\r\n
	uuid, err := peermng.GetNextSimpleStr(peer)

	if err != nil {
		return fmt.Errorf("get uuid failed%s", err.Error())
	}
	peer.RequestId = uuid
	HoldInfoLock.Lock()
	if _, ok := HoldInfo[uuid]; ok {
		HoldInfo[uuid].FunPeer = peer
	} else {
		HoldInfo[uuid] = &HoldPair{
			TermPeer:   peer,
			CreateTime: time.Now(),
			uuid:       uuid,
		}
	}
	HoldInfoLock.Unlock()
	for i := 0; i < 15; i++ {
		if HoldInfo[uuid].FunPeer != nil {
			return cmdProxyLoop(peer, uuid)
			// process
		}
		time.Sleep(1 * time.Second)
	}
	return makeActionResult(peer, uuid, string(Failed))
}
func srvFunHold(peer *peermng.Peer) error {
	//format:+action\r\n+uuid\r\n
	uuid, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get uuid failed%s", err.Error())
	}
	peer.RequestId = uuid
	HoldInfoLock.Lock()
	if _, ok := HoldInfo[uuid]; ok {
		HoldInfo[uuid].FunPeer = peer
	} else {
		HoldInfo[uuid] = &HoldPair{
			FunPeer:    peer,
			CreateTime: time.Now(),
			uuid:       uuid,
		}
	}
	HoldInfoLock.Unlock()
	return nil

}

func srvFunCmdRsp(peer *peermng.Peer) error {
	//format:+action\r\n+jsonRsp\r\n
	jsonRsp, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get jsonReq failed%s", err.Error())
	}
	err = makeActionResult(HoldInfo[peer.RequestId].TermPeer, peer.RequestId, jsonRsp)
	if err != nil {
		return fmt.Errorf("send makeActionResult action failed%s", err.Error())
	}
	return nil
}

func funCmdReq(peer *peermng.Peer) error {
	//format:+action\r\n+json req\r\n
	jsonReq, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get jsonReq failed%s", err.Error())
	}

	jsonResp, err := json.Marshal(getHoldOnRsp(jsonReq))
	if err != nil {
		return fmt.Errorf("marshal getHoldOnRsp failed%s", err.Error())
	}
	fmt.Printf("Processing holdon request %+v, rsp:%+v\n", jsonReq, jsonResp)

	_, err = desp.SendSimpleStr(string(SrvFunCmdRsp), peer.Conn)
	if err != nil {
		return fmt.Errorf("send SrvFunCmdRsp action failed%s", err.Error())
	}

	_, err = desp.SendSimpleStr(string(jsonResp), peer.Conn)
	if err != nil {
		return fmt.Errorf("send jsonResp failed%s", err.Error())
	}
	return nil

}
func srvRcvFile(peer *peermng.Peer) error {
	peer.Type = peermng.Function
	//format:+action\r\n+uuid\r\n+filename\r\nfilecontent
	// now peer.CurIdx point to "+uuid"
	uuid, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get uuid failed%s", err.Error())
	}
	uuidSet[uuid] = true
	defer func(uuid string) {
		delete(uuidSet, uuid)
	}(uuid)

	fileName, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get fileName failed%s", err.Error())
	}
	fmt.Printf("starting download file:%s, new name:%s\n", fileName, uuid)
	path := fmt.Sprintf("/tmp/%s", uuid)
	return rcvFile(path, peer)
}

const sndTimeOut = 900

func srvSndFile(peer *peermng.Peer) error {
	peer.Type = peermng.Term
	//format:+action\r\n+uuid\r\n
	// now peer.CurIdx point to "+uuid"
	uuid, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get uuid failed%s", err.Error())
	}

	name, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get file name failed%s", err.Error())
	}
	path := fmt.Sprintf("/tmp/%s", uuid)
	var i int
	var timeout bool = false
	for i = 0; i < sndTimeOut; i++ {
		time.Sleep(1 * time.Second)
		_, err := os.Stat(path)
		if err != nil {
			if i > 15 {
				fmt.Printf("finish check path err:%s\n", err.Error())
				timeout = true
				break
			}
			fmt.Printf("check path err:%s\n", err.Error())
			continue
		}
		if _, ok := uuidSet[uuid]; ok {
			if i%10 == 0 {
				fmt.Printf("dont finish rcv\n")
			}
			continue
		}
		//timeout = true
		break
	}
	_, err = desp.SendSimpleStr(string(CliRcvFile), peer.Conn)
	if err != nil {
		return err
	}

	_, err = desp.SendSimpleStr(uuid, peer.Conn)
	if err != nil {
		return err
	}
	if timeout || i == sndTimeOut {
		_, err = desp.SendSimpleStr(string(Failed), peer.Conn)
		return err
	}

	_, err = desp.SendSimpleStr(string(Success), peer.Conn)
	if err != nil {
		return err
	}

	_, err = desp.SendSimpleStr(name, peer.Conn)
	if err != nil {
		return err
	}

	return SendContent(path, peer.Conn)
}

func cliRcvFile(peer *peermng.Peer) error {
	peer.Type = peermng.Server
	//format:+action\r\n+uuid\r\n+result\r\n[+name\r\nfilecontent]
	// now peer.CurIdx point to "+uuid"
	_, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get uuid failed%s", err.Error())
	}

	result, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get result failed%s", err.Error())
	}

	name, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get file name failed%s", err.Error())
	}

	if result != string(Success) {
		return fmt.Errorf("download %s failed%s", name, result)
	}
	return rcvFile(name, peer)
}

func SendContent(path string, conn net.Conn) error {
	fs, err := os.Open(path)
	defer fs.Close()
	if err != nil {
		return fmt.Errorf("open path:%v failed:%v", path, err.Error())
	}

	fmt.Printf("start sending %s...\n", path)
	startTime := time.Now()
	buf := make([]byte, 1024*1024*5)
	for {
		//  打开之后读取文件
		n, err := fs.Read(buf)
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Printf("send:%s success,cost:%s", path, time.Since(startTime))
				return nil
			}
			return fmt.Errorf("read file:%v failed:%v", path, err.Error())

		}

		//  发送文件
		wn, err := conn.Write(buf[:n])
		if err != nil {
			return fmt.Errorf("conn write failed:%v", err.Error())
		}
		if wn != n {
			return fmt.Errorf("send:%d real:%d", wn, n)
		}
	}

}

func rcvFile(path string, peer *peermng.Peer) error {

	fs, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file:%s,failed:%s", path, err.Error())
	}
	defer fs.Close()

	// first we write remain data in peer.Buf into file
	fs.Write(peer.Buf[peer.CurIdx:peer.NxtIdx])

	startTime := time.Now()
	// then we get data in a loop, and no longer maintain peer.CurIdx and peer.NxtIdx
	for {
		n, err := peermng.ReadMoreData(peer.Buf, peer.Conn)
		if err != nil {
			if err == io.EOF {
				fmt.Printf("\033[1;32;40mdownload file:%s success,local cost:%s\033[0m\n", path, time.Since(startTime))

				return nil
			}
			return fmt.Errorf("rcvfile read more data err:%s", err.Error())
		}
		fs.Write(peer.Buf[:n])
	}
}

func processData(peer *peermng.Peer) error {
	action, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return fmt.Errorf("get action failed:%s", err.Error())
	}
	fmt.Printf("get action:%s success\n", action)
	peer.TcpAction = action
	if actionFun, ok := TcpActionTable[TcpAction(peer.TcpAction)]; ok {
		return actionFun(peer)
	}

	return fmt.Errorf("invalid action:[%v]", peer.TcpAction)
}

func getActionResult(peer *peermng.Peer) (string, error) {
	action, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return "", fmt.Errorf("get action failed:%s", err.Error())
	}
	if action != ActionResult {
		return "", fmt.Errorf("getActionResult get expected action:%s", action)
	}
	_, err = peermng.GetNextSimpleStr(peer)
	if err != nil {
		return "", fmt.Errorf("get uuid failed:%s", err.Error())
	}
	result, err := peermng.GetNextSimpleStr(peer)
	if err != nil {
		return "", fmt.Errorf("get uuid failed:%s", err.Error())
	}
	return result, nil

}

func HoldOnServer(peer *peermng.Peer) error {
	_, err := desp.SendSimpleStr(string(SrvTermHold), peer.Conn)
	if err != nil {
		return err
	}

	_, err = desp.SendSimpleStr(peer.RequestId, peer.Conn)
	if err != nil {
		return err
	}
	result, err := getActionResult(peer)
	if err != nil {
		return err
	}
	if result != string(Success) {
		return fmt.Errorf("get failed result:%s", result)
	}
	return nil
}

//SrvTermCmd
func ProxyCmd(peer *peermng.Peer, req string) (string, error) {
	_, err := desp.SendSimpleStr(string(SrvTermCmd), peer.Conn)
	if err != nil {
		return "", err
	}

	_, err = desp.SendSimpleStr(peer.RequestId, peer.Conn)
	if err != nil {
		return "", err
	}

	_, err = desp.SendSimpleStr(req, peer.Conn)
	if err != nil {
		return "", err
	}
	result, err := getActionResult(peer)
	if err != nil {
		return "", err
	}
	return result, nil
}
