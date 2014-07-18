package main
import (
	"crypto/tls"
	"net"
	"log"
	"github.com/XVManage/Node/vm"
	"github.com/XVManage/Node/util"
)

func main() {
	util.LoadConfig()

	vm.InitializeLibvirt()
	
	nodeListener, err := net.Listen("tcp4", "0.0.0.0:1532")
	if err != nil {
		log.Panicf("Node API: cannot listen: %v", err)
	}
	nodeListener = tls.NewListener(nodeListener, util.GetSslConfig())
	log.Println("Node API: ready for commands")
	
	for {
		nodeConn, err := nodeListener.Accept()
		if nodeConn == nil {
			log.Printf("accept error: %v\n", err)
		} else {
			go handleNodeConn(nodeConn)
		}
	}
}
