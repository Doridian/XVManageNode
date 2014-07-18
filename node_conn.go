package main
import (
	"net"
	"io"
	"log"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"github.com/XVManage/Node/vm"
	"github.com/XVManage/Node/util"
)

type APIRequest struct {
	Key string
	Target string
	Action string
	
	Vm string
	
	Ssl bool
}

type APICommandResult struct {
	Result string
}

type APIStatusResult struct {
	Result vm.VMStatus
}

type APIListResult struct {
	Result []vm.VMStatus
}

type APIVNCResult struct {
	Password string
	Port int64
}

func handleNodeConn(nodeConn net.Conn) {
	defer nodeConn.Close()

	buf := make([]byte, 4)
	io.ReadFull(nodeConn, buf)
	bufLenByteBuf := bytes.NewBuffer(buf)
	var bufLen int32
	binary.Read(bufLenByteBuf, binary.BigEndian, &bufLen)
	
	buf = make([]byte, bufLen)
	io.ReadFull(nodeConn, buf)

	var apiRequest APIRequest
	err := json.Unmarshal(buf, &apiRequest)
	if err != nil {
		log.Printf("API request: json err: %v", err)
		return
	}
	
	if apiRequest.Key != util.GetApiKey() {
		log.Printf("Wrong API key: %v", apiRequest.Key)
		return
	}
	
	//log.Printf("API request: %v", apiRequest)
	
	res := processAPIRequest(apiRequest)
	if res == nil {
		return
	}
	
	resBytes, err := json.Marshal(res)
	if err != nil {
		log.Printf("API reply: json err: %v", err)
		return
	}
	
	bufLenByteBuf = new(bytes.Buffer)
	err = binary.Write(bufLenByteBuf, binary.BigEndian, int32(len(resBytes)))
	if err != nil {
		log.Printf("API Buf error: %v", err)
	}
	buf = bufLenByteBuf.Bytes()
	nodeConn.Write(buf)
	nodeConn.Write(resBytes)
}

func processAPIRequest(apiRequest APIRequest) interface{} {
	if apiRequest.Target == "vm" {
		switch apiRequest.Action {
			case "list":
				var result APIListResult
				result.Result = vm.List()
				return result
			case "vnc":
				vncPort := vm.GetVNCPort(apiRequest.Vm)
				if vncPort < 1 {
					break
				}
				var result APIVNCResult
				result.Password = util.RandString(16)
				result.Port = vm.ProxyVNC(vncPort, result.Password, apiRequest.Ssl)
				return result
			case "status":
				var result APIStatusResult
				result.Result = vm.GetStatus(apiRequest.Vm)
				return result
			default:
				var result APICommandResult
				result.Result = "OK"
				vm.ProcessCommand(apiRequest.Vm, apiRequest.Action)
				return result
		}
	}
	
	var res APICommandResult
	res.Result = "ERROR"
	return res
}
