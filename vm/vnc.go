package vm
import (
	"log"
	"encoding/xml"
	"strconv"
)

type VIRXMLGraphics struct {
	Type string `xml:"type,attr"`
	Port string `xml:"port,attr"`
}

type VIRXMLDevices struct {
	Graphics []VIRXMLGraphics `xml:"graphics"`
}

type VIRXMLRes struct {
	XMLName xml.Name `xml:"domain"`
	Devices	VIRXMLDevices `xml:"devices"`
}

func GetVNCPort(name string) int64 {
	virConn := getLibvirtConnection()
	defer virConn.UnrefAndCloseConnection()

	virDomain := getLibvirtDomain(virConn, name)
	virStrXML, _ := virDomain.GetXMLDesc(0)
	var virXML VIRXMLRes
	err := xml.Unmarshal([]byte(virStrXML), &virXML)
	if err != nil {
		log.Printf("XML dom error: %v", err)
		return 0
	}
	
	virGraphics := virXML.Devices.Graphics
	for k := range virGraphics {
		virGraphic := virGraphics[k]
		if virGraphic.Type == "vnc" {
			port, _ := strconv.ParseInt(virGraphic.Port, 10, 64)
			return port
		}
	}
	return 0
}
