package peermng

import (
	"bytes"
	"fmt"
	"net"
)

type PeerType string

const (
	Function PeerType = "Funcion"
	Term     PeerType = "Term"
	Server   PeerType = "Server"

	BufSize = 1024 * 1024
)

type Peer struct {
	RequestId      string
	Type           PeerType
	Conn           net.Conn
	Buf            []byte
	CurIdx, NxtIdx int
	TcpAction      string
}

//idx: <0:invalid; ==0:mayby need read more data; >0 next index

func GetNextSimpleStr(peer *Peer) (string, error) {
	//todo clean the buf temporarily  need test
	if peer.CurIdx*2 > BufSize && peer.CurIdx == peer.NxtIdx {
		peer.CurIdx = 0
		peer.NxtIdx = 0
	}
	var needRead bool
	for {
		// empty simple string:"+\r\n"
		if needRead || peer.NxtIdx-peer.CurIdx < 3 {
			n, err := ReadMoreData(peer.Buf[peer.NxtIdx:], peer.Conn)
			if err != nil {
				return "", err
			}

			peer.NxtIdx = peer.NxtIdx + n
		}
		needRead = false
		buf := peer.Buf[peer.CurIdx:peer.NxtIdx]
		if len(buf) <= 0 {
			needRead = true
			fmt.Printf("len of buf is 0, should not occur\n")
			continue
		}

		// for simple string,we start with "+"
		if buf[0] != '+' {
			return "", fmt.Errorf("error buf prefix")
		}

		n := bytes.Index(buf, []byte("\r\n"))
		if n <= 1 {
			needRead = true
			fmt.Printf("can not find end mark, read more....\n")
			continue
		}
		peer.CurIdx = peer.CurIdx + n + 2
		return string(buf[1:n]), nil
	}
}

func InitPeer(conn net.Conn) *Peer {
	return &Peer{
		Conn: conn,
		Buf:  make([]byte, BufSize),
	}
}

func ReadMoreData(buf []byte, conn net.Conn) (int, error) {
	n, err := conn.Read(buf)
	// if interrupted,start read again?
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, fmt.Errorf("get metadata failed, read zero byte")
	}
	return n, nil
}
