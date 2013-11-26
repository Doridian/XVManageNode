package vm
import (
	"os/exec"
	"log"
	"bufio"
	"strings"
	"regexp"
	"bytes"
	"os"
	"encoding/json"
	"reflect"
)

//29(vnet13): addr:fe:54:00:fc:56:22
var ofctlRegex = regexp.MustCompile("^([0-9]+)\\(([a-zA-Z0-9]+)\\): addr:[0-9a-f:]+$")

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

	cmd := exec.Command("sudo", "ovs-ofctl", "dump-ports-desc", "ovs0")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("ofctl error: %v", err)
		return
	}
	outputBufio := bufio.NewReader(bytes.NewReader(output))
	
	exec.Command("sudo", "/root/flows.sh").Run()
	
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
		if vmDef.vmid <= 0 {
			continue
		}
		portID := outSplit[1]
		
		allowedIPs := nodeIPs[vmDef.vmname]
		
		for _, allowedIP := range allowedIPs {
			exec.Command("sudo", "ovs-ofctl", "add-flow", "ovs0", "ip,priority=2,nw_dst=" + allowedIP + ",actions=output:" + portID).Run()
		}
		
		//log.Printf("Port %v is VM %v", portID, allowedIPs)
	}
}