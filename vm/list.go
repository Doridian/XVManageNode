package vm
import (
	"log"
	"time"
	"sync"
)

var vmDomains struct {
    sync.RWMutex
    m map[string]VMDomain
}

func maintainVMListTicker() {
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for {
			select {
				case <- ticker.C:
					maintainVMList()
			}
		}
	}()
}

func maintainVMList() {
	log.Println("Refreshing VM stats")

	virConn := getLibvirtConnection()
	defer virConn.UnrefAndCloseConnection()
	
	virDomains, err := virConn.ListDomains()
	if err != nil {
		log.Printf("Libvirt error: %v", err.Error())
		return
	}
	
	virDomainsOffline, err := virConn.ListDefinedDomains()
	if err != nil {
		log.Printf("Libvirt error: %v", err.Error())
		return
	}
	
	var vmNWParams map[string]VMNetDefinition
	vmNWParams = make(map[string]VMNetDefinition)
	
	vmDomains.Lock()
	defer vmDomains.Unlock()

	for virName, vmDomain := range vmDomains.m {
		vmDomain.removePossible = true
		vmDomains.m[virName] = vmDomain
	}
	
	for _, virDomainID := range virDomains {
		virDomain, _ := virConn.LookupDomainById(virDomainID)
		virDomainInfo, _ := virDomain.GetInfo()
		virName, _ := virDomain.GetName()
		
		vmDomain := vmDomains.m[virName]
		vmDomain.name = virName

		vmDomain.removePossible = false
		
		vmDomain.poweredOn = virDomainInfo.GetState() == 1
		
		cpuTime := virDomainInfo.GetCpuTime()
		
		nowTime := time.Now()
		if !vmDomain.lastCheck.IsZero() {
			vmDomain.cpuUsage = float64(cpuTime - vmDomain.lastCpuTime) * 100.0 / float64(nowTime.Sub(vmDomain.lastCheck).Nanoseconds())
		}
		
		vmDomain.lastCheck = nowTime
		vmDomain.lastCpuTime = cpuTime
		
		vmDomain.vcpus = int64(virDomainInfo.GetNrVirtCpu())	
		vmDomain.ramUsage = float64(virDomainInfo.GetMemory()) * 100.0 / float64(virDomainInfo.GetMaxMem())
		
		vmDomains.m[virName] = vmDomain
		
		virNWParams := *GetNWParams(virDomainID)
		virNWParams.vmname = virName
		vmNWParams[virNWParams.ifname] = virNWParams
	}
	
	for _, virName := range virDomainsOffline {
		vmDomain := vmDomains.m[virName]
		vmDomain.name = virName
		vmDomain.poweredOn = false
		vmDomain.cpuUsage = 0
		vmDomain.ramUsage = 0
		vmDomain.vcpus = 1
		vmDomain.removePossible = false
		vmDomain.lastCpuTime = 0
		vmDomain.lastCheck = time.Unix(0, 0)
		vmDomains.m[virName] = vmDomain
	}
	
	deletionList := make([]string, 0)
	
	for virName, vmDomain := range vmDomains.m {
		if vmDomain.removePossible {
			deletionList = append(deletionList, virName)
		}
	}
	
	for _, virName := range deletionList {
		delete(vmDomains.m, virName)
	}
	
	maintainVSwitch(vmNWParams)
}

type VMStatus struct {
	Name string
	IsPoweredOn bool
	CpuUsage float64
	RamUsage float64
	Vcpus int64
}

func (d *VMDomain) makeVMStatus() VMStatus {
	var vmStatus VMStatus
	vmStatus.Name = d.name
	vmStatus.IsPoweredOn = d.poweredOn
	vmStatus.CpuUsage = d.cpuUsage
	vmStatus.RamUsage = d.ramUsage
	vmStatus.Vcpus = d.vcpus
	return vmStatus
}
