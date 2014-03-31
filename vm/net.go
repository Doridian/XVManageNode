package vm
import (
	"log"
	"encoding/xml"
	"encoding/json"
	"os/exec"
	"bufio"
	"strings"
	"regexp"
	"bytes"
	"os"
	"io"
	"fmt"
	"reflect"
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
	vmid uint32
	vmname string
	ifname string

	mac2 string
	ifname2 string
}

func GetNWParams(name string, vmType string) *VMNetDefinition {
	virConn := getLibvirtConnection(vmType)
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
	iFace2 := virXML.Devices.Interfaces[1]
	
	ret := new(VMNetDefinition)
	ret.vmname = name

	ret.mac = iFace.Mac.Address
	ret.ifname = iFace.Target.Dev

	ret.mac2 = iFace2.Mac.Address
	ret.ifname2 = iFace2.Target.Dev

	ret.vmid = 0

	return ret
}

//29(vnet13): addr:fe:54:00:fc:56:22
var ofctlRegex = regexp.MustCompile("^([0-9]+)\\(([a-zA-Z0-9]+)\\): addr:([0-9a-f:]+)$")

var nodeIPs map[string][]string

var lastVMDefs map[string]VMNetDefinition

func loadIPConfig() bool {
	var newNodeIPs map[string][]string
	fileReader, err := os.Open("ips.json")
	if err != nil {
		log.Panicf("Load IPs: open err: %v", err)
	}	
	jsonReader := json.NewDecoder(fileReader)
	err = jsonReader.Decode(&newNodeIPs)
	fileReader.Close()
	if err != nil {
		log.Panicf("Load IPs: json err: %v", err)
	}
	if reflect.DeepEqual(newNodeIPs, nodeIPs) {
		return false
	}
	nodeIPs = newNodeIPs
	return true
}

func maintainVSwitch(vmDefs map[string]VMNetDefinition) {
	if !loadIPConfig() && reflect.DeepEqual(vmDefs, lastVMDefs) {
		return
	}
	
	log.Println("Refreshing VM networks")
	
	lastVMDefs = vmDefs

	//OpenVSWitch
	cmd := exec.Command("sudo", "ovs-ofctl", "dump-ports-desc", "ovs0")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("ofctl error: %v", err)
		return
	}
	outputBufio := bufio.NewReader(bytes.NewReader(output))
	
	output, err = exec.Command("sudo", "/root/flows.sh").Output()
	if err != nil {
		log.Printf("flows.sh error: %v", err)
		return
	}
	
	ethPortID := string(output)
	
	for {
		out, err := outputBufio.ReadString('\n')
		if err != nil {
			break
		}
		out = strings.Trim(out, " \r\n\t")
		outSplit := ofctlRegex.FindStringSubmatch(out)
		if outSplit == nil || len(outSplit) < 3 {
			continue
		}
		
		ifName := outSplit[2]
		vmDef := vmDefs[ifName]	
		if vmDef.vmname == "" {
			continue
		}
		portID := outSplit[1]
		
		allowedIPs := nodeIPs[vmDef.vmname]
		
		for _, allowedIP := range allowedIPs {
			exec.Command("sudo", "ovs-ofctl", "add-flow", "ovs0", "ip,priority=3,nw_dst=" + allowedIP + ",actions=mod_dl_dst=" + vmDef.mac + ",output:" + portID).Run()
			//exec.Command("sudo", "ovs-ofctl", "add-flow", "xovs0", "ip,priority=3,nw_dst=" + allowedIP + ",output:" + portID).Run()
			exec.Command("sudo", "ovs-ofctl", "add-flow", "ovs0", "ip,priority=2,nw_src=" + allowedIP + ",actions=output:" + ethPortID).Run()
		}
	}
	
	//DHCP
	dhcpConfig, _ := os.Create("/etc/dhcp/dhcpd.conf")
	dhcpHeader, _ := os.Open("dhcpd.conf.head")
	io.Copy(dhcpConfig, dhcpHeader)
	dhcpHeader.Close()
	
	for _, vmDef := range vmDefs {
		allowedIPs := nodeIPs[vmDef.vmname]
		
		if len(allowedIPs) < 1 {
			log.Printf("Warning: VM %v has no assigned IPs!!!", vmDef.vmname)
			continue
		}
		
		fmt.Fprintf(dhcpConfig, "\nhost %v_ext {\n\thardware ethernet %v;\n\tfixed-address %v;\n}\n", vmDef.vmname, vmDef.mac, allowedIPs[0])
		fmt.Fprintf(dhcpConfig, "\nhost %v_int {\n\thardware ethernet %v;\n\tfixed-address 10.88.1.%v;\n}\n", vmDef.vmname, vmDef.mac2, strings.Split(allowedIPs[0], ".")[3])
	}
	
	dhcpConfig.Close()
	
	exec.Command("sudo", "/usr/sbin/service", "isc-dhcp-server", "restart").Run()
}
