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
	"github.com/XVManage/Node/util"
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

type VMNetIfaceDefinition struct {
	mac string
	ifname string
}

type VMNetDefinition struct {
	vmid uint32
	vmname string
	ifaces []VMNetIfaceDefinition
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
	
	ret := new(VMNetDefinition)
	ret.vmname = name
	
	ret.ifaces = make([]VMNetIfaceDefinition, len(virXML.Devices.Interfaces))
	
	for i := 0; i < len(ret.ifaces); i++ {
		_virIf := virXML.Devices.Interfaces[i]
		ret.ifaces[i].mac = _virIf.Mac.Address
		ret.ifaces[i].ifname = _virIf.Target.Dev
	}

	ret.vmid = 0

	return ret
}

//29(vnet13): addr:fe:54:00:fc:56:22
var ofctlRegex = regexp.MustCompile("^([0-9]+)\\(([a-zA-Z0-9]+)\\): addr:([0-9a-f:]+)$")

var nodeIPs map[string][][]string

var lastVMDefs map[string]VMNetDefinition

func loadIPConfig() bool {
	var newNodeIPs map[string][][]string
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

func maintainInterfaces(vmDefs map[string]VMNetDefinition) {
	if !loadIPConfig() && reflect.DeepEqual(vmDefs, lastVMDefs) {
		return
	}
	
	log.Println("Refreshing VM networks")
	
	lastVMDefs = vmDefs

	//OpenVSWitch
	_, err := exec.Command("sudo", "/root/flows.sh").Output()
	if err != nil {
		log.Printf("flows.sh error: %v", err)
		return
	}
	
	ifaceConfigs := util.GetAllInterfaceConfigs()
	for ifaceNum, ifaceConfig := range ifaceConfigs {
		switch ifaceConfig.Type {
			case "ovs":
				maintainVSwitch(vmDefs, ifaceNum, ifaceConfig.Master)
			case "bridge":
				maintainBridge(vmDefs, ifaceNum, ifaceConfig.Master)
		}
	}
	
	//DHCP
	dhcpConfig, _ := os.Create("/etc/dhcp/dhcpd.conf")
	dhcpHeader, _ := os.Open("dhcpd.conf.head")
	io.Copy(dhcpConfig, dhcpHeader)
	dhcpHeader.Close()
	
	triedIfNames := make(map[string]bool)
	
	for _, vmDef := range vmDefs {		
		for vmIfNum, vmIfDef := range vmDef.ifaces {
			ifName := fmt.Sprintf("%v_eth%v", vmDef.vmname, vmIfNum)
			if triedIfNames[ifName] {
				continue
			}
			triedIfNames[ifName] = true
		
			if vmIfNum >= len(nodeIPs[vmDef.vmname]) {
				log.Printf("Warning: VM if %v has no assigned IPs!!!", ifName)
				continue
			}
			
			allowedIPs := nodeIPs[vmDef.vmname][vmIfNum]
		
			if len(allowedIPs) < 1 {
				log.Printf("Warning: VM if %v has no assigned IPs!!!", ifName)
				continue
			}
			
			fmt.Fprintf(dhcpConfig, "\nhost %v {\n\thardware ethernet %v;\n\tfixed-address %v;\n}\n", ifName, vmIfDef.mac, allowedIPs[0])
		}
	}
	
	dhcpConfig.Close()
	
	exec.Command("sudo", "/usr/sbin/service", "isc-dhcp-server", "restart").Run()
}

func maintainBridge(vmDefs map[string]VMNetDefinition, number int, master string) {
}

func maintainVSwitch(vmDefs map[string]VMNetDefinition, number int, master string) {
	cmd := exec.Command("sudo", "ovs-ofctl", "dump-ports-desc", master)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("ofctl error: %v", err)
		return
	}
	outputBufio := bufio.NewReader(bytes.NewReader(output))
	
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
		
		allowedIPs := nodeIPs[vmDef.vmname][number]
		
		for _, allowedIP := range allowedIPs {
			exec.Command("sudo", "ovs-ofctl", "add-flow", master, "ip,priority=3,nw_dst=" + allowedIP + ",actions=mod_dl_dst=" + vmDef.ifaces[number].mac + ",output:" + portID).Run()
			//exec.Command("sudo", "ovs-ofctl", "add-flow", master, "ip,priority=3,nw_dst=" + allowedIP + ",output:" + portID).Run()
			exec.Command("sudo", "ovs-ofctl", "add-flow", master, "ip,priority=2,nw_src=" + allowedIP + ",actions=output:" + ethPortID).Run()
		}
	}
}
