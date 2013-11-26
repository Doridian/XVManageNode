package vm
import (
	"log"
	"encoding/xml"
)

type VIRXMLMac struct {
	Address string `xml:"address,attr"`
}

type VIRXMLTargetM struct {
	Dev string `xml:"dev,attr"`
}

type VIRXMLInterface struct {
	Mac VIRXMLMac `xml:"mac"`
	Target VIRXMLTargetM `xml:"target"`
}

type VIRXMLDevicesM struct {
	Interfaces []VIRXMLInterface `xml:"interface"`
}

type VIRXMLResM struct {
	XMLName xml.Name `xml:"domain"`
	Devices	VIRXMLDevicesM `xml:"devices"`
}

type VMNetDefinition struct {
	mac string
	vmname string
	ifname string
}

func GetNWParams(name string) *VMNetDefinition {
	virConn := getLibvirtConnection()
	defer virConn.UnrefAndCloseConnection()

	virDomain := getLibvirtDomain(virConn, name)
	virStrXML, _ := virDomain.GetXMLDesc(0)
	var virXML VIRXMLResM
	err := xml.Unmarshal([]byte(virStrXML), &virXML)
	if err != nil {
		log.Printf("XML dom error: %v", err)
		return nil
	}
	
	iFace := virXML.Devices.Interfaces[0]
	
	ret := new(VMNetDefinition)
	ret.vmname = name
	ret.mac = iFace.Mac.Address
	ret.ifname = iFace.Target.Dev
	return ret
}
