package util
import (
	"crypto/tls"
	"encoding/json"
	"os"
	"log"
	"sync"
)

//Global SSL/TLS config
var sslCertificates tls.Certificate

//Global Node config
var nodeConfig NodeConfig

type NodeConfig struct {
	ApiKey string
}

var sslConfigMutex sync.Mutex
func GetSslConfig() *tls.Config {
	sslConfigMutex.Lock()
	defer sslConfigMutex.Unlock()
	sslConfig := new(tls.Config)
	sslConfig.Certificates = []tls.Certificate{sslCertificates}
	return sslConfig
}

func GetApiKey() string {
	return nodeConfig.ApiKey
}

func LoadConfig() {
	fileReader, err := os.Open("config.json")
	if err != nil {
		log.Panicf("Load Config: open err: %v", err)
	}	
	jsonReader := json.NewDecoder(fileReader)
	err = jsonReader.Decode(&nodeConfig)
	fileReader.Close()
	if err != nil {
		log.Panicf("Load Config: json err: %v", err)
	} else {
		log.Println("Load Config: OK")
	}

	sslCertificates, err = tls.LoadX509KeyPair("node.crt", "node.key")
	if err != nil {
		log.Panicf("Load SSL: error %v", err)
	} else {
		log.Println("Load SSL: OK")
	}
}
