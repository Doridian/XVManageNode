package vm
import (
	"bytes"
	"net"
	"io"
	"log"
	"time"
	"strconv"
	"crypto/rand"
	"crypto/des"
	"crypto/tls"
	"sync/atomic"
	"github.com/XVManage/Node/util"
)

//Gets a free port (might need more smartness soon or something)
var port_counter int64
func getListenPort() int64 {
	return (atomic.AddInt64(&port_counter, 1) % 1000) + 19000
}

func ProxyVNC(vncPort int64, password string, useSSL bool) int64 {
	listenPort := getListenPort()
	go listenVNC(listenPort, vncPort, password, useSSL)
	return listenPort
}

//Listen for exactly one VNC connection on specified target port (ready to proxy to local VNC)
func listenVNC(listenPort int64, vncPort int64, password string, useSSL bool) {
	listenAddr := "0.0.0.0:" + strconv.FormatInt(listenPort, 10)

	//We use all the ListenTCP here to be able to use TCPListener.SetDeadline
	listenTcpAddr, err := net.ResolveTCPAddr("tcp4", listenAddr)
	if listenTcpAddr == nil {
		log.Printf("cannot resolve: %v %s", err, listenTcpAddr)
		return
	}

	tcpListener, err := net.ListenTCP("tcp4", listenTcpAddr)
	if tcpListener == nil {
		log.Printf("cannot listen: %v %s", err, listenAddr)
		return
	}
	tcpListener.SetDeadline(time.Now().Add(60 * time.Second))

	//Wrap SSL/TLS if requested
	listener := net.Listener(tcpListener)
	if useSSL {
		listener = tls.NewListener(listener, util.GetSslConfig())
	}

	//Accept one connection, then close listener
	clientConn, err := listener.Accept()
	listener.Close()
	if clientConn == nil {
		log.Printf("accept failed: %v $s", err, listenAddr)
		return
	}

	handleAuth(clientConn, vncPort, password)
}

//Handle VNC authentication and handshaking from the remote
func handleAuth(clientConn net.Conn, vncPort int64, password string) {
	passwordBytes := make([]byte, 8)
	copy(passwordBytes, []byte(password))

	io.WriteString(clientConn, "RFB 003.008\n")
	buf := make([]byte, 12)
	io.ReadFull(clientConn, buf)

	//1 auth method present, type 2 (VNC authentication)
	clientConn.Write([]byte{1, 2})
	//Read auth method response, only accept type 2
	buf = make([]byte, 1)
	io.ReadFull(clientConn, buf)
	if buf[0] != 2 {
		clientConn.Close()
		log.Printf("wrong auth type: %v", buf)
		return
	}

	//Make challenge for VNC authentication
	challenge := make([]byte, 16)
	rand.Read(challenge)
	clientConn.Write(challenge)
	
	//Read response
	response := make([]byte, 16)
	io.ReadFull(clientConn, response)

	//VNC mirrors bits in the password used for DES (http://www.vidarholen.net/contents/junk/vnc.html)
	mirrorBits(passwordBytes)
	//Create cipher from password (VNC auth = DES encrypt challenge with password)
	responseCipher, err := des.NewCipher(passwordBytes)
	if responseCipher == nil {
		clientConn.Close()	
		log.Printf("cipher failed: %v", err)
		return
	}

	//Decrypt the two blocks of the challenge
	challengeDecrypted := make([]byte, 16)
	responseCipher.Decrypt(challengeDecrypted[:8], response[:8])
	responseCipher.Decrypt(challengeDecrypted[8:], response[8:])

	//Compare challenge with decrypted challenge
	if bytes.Equal(challengeDecrypted, challenge) {
		forward(clientConn, vncPort)
	} else {
		//U32 for "failed" (0 is okay), then U32 for length of reason
		clientConn.Write([]byte{0,0,0,1, 0,0,0,14})
		io.WriteString(clientConn, "Wrong password") //len = 5 + 1 + 8 = 14
		clientConn.Close()
		log.Printf("auth failed %v %v %v\n", challenge, challengeDecrypted, passwordBytes)
		return
	}
}

//Establish connection to target VNC server and do basic handshaking
func forward(clientConn net.Conn, vncPort int64) {
	vncAddr := "127.0.0.1:" + strconv.FormatInt(vncPort, 10)
	vncConn, err := net.Dial("tcp", vncAddr)
	if vncConn == nil {
		clientConn.Close()
		log.Printf("remote dial failed: %v", err)
		return
	}

	//Read version string from VNC server
	buf := make([]byte, 12)
	io.ReadFull(vncConn, buf)
	//Reply with v3.8 (the version we use)
	io.WriteString(vncConn, "RFB 003.008\n")

	//Read 1 byte (count of auth methods)
	buf = make([]byte, 1)
	io.ReadFull(vncConn, buf)
	//Read auth methods
	buf = make([]byte, buf[0])
	io.ReadFull(vncConn, buf)
	
	//use auth method 1 (no authentication)
	vncConn.Write([]byte{1})

	//Real proxying here
	go io.Copy(clientConn, vncConn)
	go io.Copy(vncConn, clientConn)
}

//Bit mirroring function for DES encryption that VNC uses
func mirrorBits(k []byte) {
	s := byte(0)
	kSize := len(k)
	for i := 0; i < kSize; i++ {
		s = k[i]
		s = (((s >> 1) & 0x55) | ((s << 1) & 0xaa))
		s = (((s >> 2) & 0x33) | ((s << 2) & 0xcc))
		s = (((s >> 4) & 0x0f) | ((s << 4) & 0xf0))
		k[i] = s
	}
}
