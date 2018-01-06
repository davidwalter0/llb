package ipmgr

import (
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/davidwalter0/go-mutex"
	"github.com/davidwalter0/llb/tracer"

	"github.com/vishvananda/netlink"
)

var _InTest_ bool

var monitor = mutex.NewMonitor()

// LinkAddr manage links track in use Count
type LinkAddr struct {
	*netlink.Addr
	Count int
}

// LoadBalancerIPs load balancer IPs
type LoadBalancerIPs map[string]*LinkAddr

// AddAddr adds an address to a network LinkDevice
func (mips *LoadBalancerIPs) AddAddr(IPNet, LinkDevice string) {
	if net.ParseIP(strings.Split(IPNet, "/")[0]) == nil {
		if Debug {
			log.Printf("AddAddr skipping invalid IPAddr:%v on LinkDevice:%v\n", IPNet, LinkDevice)
		}
		return
	}

	log.Printf("AddAddr %v %v\n", IPNet, LinkDevice)
	defer monitor()()
	defer trace.Tracer.ScopedTrace()()
	if Debug {
		for key := range *mips {
			log.Println(key)
		}
	}
	if linkAddr, ok := (*mips)[IPNet]; !ok {
		if link, err := netlink.LinkByName(LinkDevice); err == nil {
			if Debug {
				log.Printf("AddAddr %v %v link: %v\n", IPNet, LinkDevice, link)
			}
			if addr, err := netlink.ParseAddr(IPNet); err == nil {
				linkAddr = &LinkAddr{Addr: addr, Count: 1}
				if Debug {
					log.Printf("AddAddr %v %v LinkAddr: %v\n", IPNet, LinkDevice, *linkAddr)
				}
				if !_InTest_ {
					if err := netlink.AddrAdd(link, addr); err == nil {
						(*mips)[IPNet] = linkAddr
					} else {
						if Debug {
							log.Println("Warning: managing existing ip", IPNet, LinkDevice)
							log.Println(err)
						}
						(*mips)[IPNet] = linkAddr
					}
				}
				if Debug {
					log.Printf("AddAddr %v %v LinkAddr: %v Count: %d\n", IPNet, LinkDevice, *linkAddr, linkAddr.Count)
				}
			} else {
				log.Println(err)
			}
		} else {
			log.Println(err)
		}
	} else {
		linkAddr.Count++
	}
}

// RemoveAddr from networks
func (mips *LoadBalancerIPs) RemoveAddr(IPNet, LinkDevice string) {
	if net.ParseIP(strings.Split(IPNet, "/")[0]) == nil {
		if Debug {
			log.Printf("AddAddr skipping invalid IPAddr:%v on LinkDevice:%v\n", IPNet, LinkDevice)
		}
		return
	}
	log.Printf("RemoveAddr %v %v\n", IPNet, LinkDevice)
	if DefaultCIDR.String() == IPNet {
		log.Printf("RemoveAddr Skips IPNet/CIDR rule %v on device %v\n", IPNet, LinkDevice)
		return
	}
	defer monitor()()
	defer trace.Tracer.ScopedTrace()()
	if Debug {
		for key := range *mips {
			log.Println(key)
		}
	}
	if linkAddr, ok := (*mips)[IPNet]; ok {
		if Debug {
			log.Printf("RemoveAddr %v %v LinkAddr: %v Count: %d\n", IPNet, LinkDevice, *linkAddr, linkAddr.Count)
		}
		addr := linkAddr.Addr
		linkAddr.Count--
		if linkAddr.Count <= 0 {
			if Debug {
				log.Println("RemoveAddr", addr, ok)
			}
			if link, err := netlink.LinkByName(LinkDevice); err == nil {
				if Debug {
					log.Printf("RemoveAddr %v %v link: %v\n", IPNet, LinkDevice, link)
					log.Println(addr, link)
				}
				if link != nil && !_InTest_ {
					if err := netlink.AddrDel(link, addr); err != nil {
						log.Println(err)
					}
				}
			} else {
				log.Println(err)
			}
			delete(*mips, IPNet)
		}
	}
}

// keys of managed ip map | not thread safe
func (mips *LoadBalancerIPs) keys() (IPNets []string) {
	for key := range *mips {
		IPNets = append(IPNets, key)
	}
	return
}

// Keys of managed ip map | thread safe
func (mips *LoadBalancerIPs) Keys() (IPNets []string) {
	defer monitor()()
	return mips.keys()
}

// String from managed ip map | thread safe
func (mips *LoadBalancerIPs) String() string {
	defer monitor()()
	return fmt.Sprintf("%v", mips.keys())
}
