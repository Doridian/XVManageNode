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
	vmid uint32
	mac string
	vmname string
	ifname string
}

//Returns (mac, devname)
func GetNWParams(id uint32) *VMNetDefinition {
	virConn := getLibvirtConnection()
	defer virConn.UnrefAndCloseConnection()

	virDomain := getLibvirtDomainByID(virConn, id)
	virStrXML, _ := virDomain.GetXMLDesc(0)
	var virXML VIRXMLResM
	err := xml.Unmarshal([]byte(virStrXML), &virXML)
	if err != nil {
		log.Printf("XML dom error: %v", err)
		return nil
	}
	
	iFace := virXML.Devices.Interfaces[0]
	
	ret := new(VMNetDefinition)
	ret.vmid = id
	ret.mac = iFace.Mac.Address
	ret.ifname = iFace.Target.Dev
	return ret
}
