package ipcc

import (
	consts "../consts"
	"io"
	"log"
	"net"
	mes "../message"
	"encoding/json"
	"sync"
	"math"
)

//
type IpcClientController struct {
	sockFiles  string
	conn       	net.Conn
	ipcrecv_ch 	chan interface{}
	call_send_ch 	chan sendInfo
	rcv_read_ch 	chan []byte
	snd_write_ch 	chan []byte
	sync_mapMutex   *sync.Mutex
	sync_map	map[uint64]chan interface{}
	seqMutex	*sync.Mutex
	seqno		uint64
}

//
func (ipcc *IpcClientController) receiver(r io.Reader, ch chan []byte) {

	_buf := make([]byte, consts.BUFF_MAX)
	for {
		n, err := r.Read(_buf[:])
		if err != nil {
			log.Println(err)
			return
		}
		select {
			case ch <- _buf[0:n]:
		}
	}
}

//
func (ipcc *IpcClientController) sender(c net.Conn, ch chan []byte ) {

	for {
		var _msg []byte
		//
		select {
			case _msg = <- ch:
		}
		_, err := c.Write(_msg)
		if err != nil {
			log.Println(err)
		}
	}
}

//
/*
func (ipcc *IpcClientController) TestPrint() {
	fmt.Println("IpcClientController")
}
*/

//
func (ipcc *IpcClientController) Connect() net.Conn {
	_c, err := net.Dial("unix", ipcc.sockFiles)
	if err != nil {
		log.Println(err)
		return nil
	}
	ipcc.conn = _c
	return _c
}

//
func (ipcc *IpcClientController) Disconnect() {
	if ipcc.conn != nil {
		if err := ipcc.conn.Close(); err != nil {
			log.Println(err)
		}
		ipcc.conn = nil	
	}
}

//
func (ipcc*IpcClientController) _setSyncMap(w sendInfo) {
	if w.sch != nil {
		ipcc.sync_mapMutex.Lock()
		defer ipcc.sync_mapMutex.Unlock()
		if _, ok := ipcc.sync_map[w.seqno]; !ok {
			ipcc.sync_map[w.seqno] = w.sch
		}
	}
}

//
func (ipcc*IpcClientController) _removeSyncMap(seqno uint64) {
	ipcc.sync_mapMutex.Lock()
	defer ipcc.sync_mapMutex.Unlock()
	delete(ipcc.sync_map, seqno)

}

//
func (ipcc*IpcClientController) GetSeqno() uint64 {
	
	ipcc.seqMutex.Lock()
	if ipcc.seqno == math.MaxUint64 {
		ipcc.seqno = 0	
	}
	ipcc.seqno = ipcc.seqno + 1
	defer ipcc.seqMutex.Unlock()
	return ipcc.seqno
	
}

//
type sendInfo struct {
	seqno uint64
	msg []byte
	sch chan interface{}
}

//
func (ipcc *IpcClientController) SendRecvAsync(msgs []byte) int {
	var send = sendInfo {
		0,
		msgs,
		nil,
	}	

	select {
		case ipcc.call_send_ch <- send : 
	}
	return consts.CL_OK
}

//
func (ipcc *IpcClientController) SendRecv(msgs []byte, timeout int64, seqno uint64) int {

	var _rcv interface{}

	_c := make(chan interface{})

	var send = sendInfo {
		seqno,
		msgs,
		_c,
	}	
	
	// send to channel...
	select {
		case ipcc.call_send_ch <- send: 
	}

	// wait Sync..
	select {
		case _rcv = <- _c:
	}

	ipcc.ipcrecv_ch <- _rcv

	return consts.CL_OK
}

//
type IpcClientMsg struct {
	Head mes.MessageHeader
	All []byte
}
//
func (ipcc *IpcClientController) Run() {

	// Start receiver.
	go ipcc.receiver(ipcc.conn, ipcc.rcv_read_ch)

	// Start sender
	go ipcc.sender(ipcc.conn, ipcc.snd_write_ch)
	//
	go func() {
		for {
			select {
				case _r  := <- ipcc.rcv_read_ch:
					var _head mes.MessageCommon
					if err := json.Unmarshal(_r, &_head); err != nil {	
						log.Println(err)
					}
					//
					
					if _v, ok := ipcc.sync_map[_head.Header.SeqNo]; ok {
						// Remove map
						ipcc._removeSyncMap(_head.Header.SeqNo)
						//Sync Result to channel.
						_v <- IpcClientMsg { _head.Header, _r }
					} else {
						//Async Result to channel.
						ipcc.ipcrecv_ch <-  IpcClientMsg { _head.Header, _r }
					}
				case _w  := <- ipcc.call_send_ch:
					//
					ipcc._setSyncMap(_w)
					ipcc.snd_write_ch <- _w.msg
			}
		}
	}()
	
}

//
func (ipcc *IpcClientController) GetReceiveChannel() chan interface{} {
	return ipcc.ipcrecv_ch
}

//
func New(sf string) *IpcClientController {
	return &IpcClientController{
		sockFiles:  sf,
		ipcrecv_ch: make(chan interface{}, 12),
		call_send_ch: make(chan sendInfo , 12),
		rcv_read_ch: make(chan []byte, 12),
		snd_write_ch: make(chan []byte, 12),
		sync_mapMutex : new(sync.Mutex),
		sync_map: make(map[uint64]chan interface{}),
		seqMutex : new(sync.Mutex),
	}
}
