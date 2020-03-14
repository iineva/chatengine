package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"
)

const (
	SERVER_IP       = "0.0.0.0"
	SERVER_PORT     = 50001
	SERVER_RECV_LEN = 1500
)

const (
	TLID_DECRYPTED_AUDIO_BLOCK        = uint32(0xDBF948C1)
	TLID_SIMPLE_AUDIO_BLOCK           = uint32(0xCC0D0E76)
	TLID_UDP_REFLECTOR_PEER_INFO      = uint32(0x27D9371C)
	TLID_UDP_REFLECTOR_PEER_INFO_IPV6 = uint32(0x83fc73b1)
	TLID_UDP_REFLECTOR_SELF_INFO      = uint32(0xc01572c7) // ping-pong IPv4
)

var PacketTypeUint32Max = NewWithUint32(0xFFFFFFFF)

func main() {

	serverOddr := SERVER_IP + ":" + strconv.Itoa(SERVER_PORT)
	addrObj, err := net.ResolveUDPAddr("udp", serverOddr)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	conn, err := net.ListenUDP("udp", addrObj)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer conn.Close()

	log.Println("======== SERVER STARTED ========")
	log.Printf("======== ADDRESS: %v ========", addrObj)
	for {

		log.Printf("======== RUN LOOP STARTED ========")

		// Here must use make and give the lenth of buffer
		buf := make([]byte, SERVER_RECV_LEN)
		len, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println(err)
			continue
		}

		//strData := string(data)
		log.Printf("Received data len: %v", len)

		if len < 32 {
			log.Println("Packet len to small")
			continue
		}
		go handlerPacket(conn, PacketSlice(buf[:len]).Clone(), addr)
	}
}

type PacketSlice []byte

func NewWithUint64(val uint64) PacketSlice {
	p := make(PacketSlice, 8)
	binary.LittleEndian.PutUint64(p, val)
	return p
}

func NewWithUint32(val uint32) PacketSlice {
	p := make(PacketSlice, 4)
	binary.LittleEndian.PutUint32(p, val)
	return p
}

func (t PacketSlice) String() string {
	return hex.EncodeToString([]byte(t))
}

func (t PacketSlice) Uint32() uint32 {
	return binary.LittleEndian.Uint32(t[0:4])
}

func (t PacketSlice) Uint64() uint64 {
	return binary.LittleEndian.Uint64(t[0:8])
}

func (t PacketSlice) Equal(t2 PacketSlice) bool {
	if len(t) != len(t2) {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] != t2[i] {
			return false
		}
	}
	return true
}

func (t PacketSlice) Clone() []byte {
	p := make(PacketSlice, len(t))
	copy(p, t)
	return p
}

func (t PacketSlice) Copy(t2 PacketSlice, offset int) PacketSlice {
	for i := 0; i < len(t2); i++ {
		t[i+offset] = t2[i]
	}
	return t
}

func handlerPacket(conn *net.UDPConn, buf PacketSlice, addr *net.UDPAddr) {

	packetTypes := make([]PacketSlice, 4)

	peerTag := PacketSlice(buf[:16])
	for i := 0; i < 4; i++ {
		offset := 16 + (i * 4)
		packetTypes[i] = buf[offset : offset+4]
	}

	if packetTypes[0].Uint32() == 0xFFFFFFFF &&
		packetTypes[1].Uint32() == 0xFFFFFFFF &&
		packetTypes[2].Uint32() == 0xFFFFFFFF &&
		packetTypes[3].Uint32() == 0xFFFFFFFF {
		// onPublicEndpointsRequest
		onPublicEndpointsRequest(conn, addr, peerTag)
	} else if packetTypes[0].Uint32() == 0xFFFFFFFF &&
		packetTypes[1].Uint32() == 0xFFFFFFFF &&
		packetTypes[2].Uint32() == 0xFFFFFFFF &&
		packetTypes[3].Uint32() == 0xFFFFFFFE {
		onUdpPing(conn, addr, peerTag, PacketSlice(buf[32:32+8]))
	} else {
		// TODO(@benqi): length invalid check
		log.Println("*****************")
		onRelayDataAvailable(conn, addr, peerTag, buf)
	}

	log.Printf("peerTag: %v", hex.EncodeToString(peerTag))
	log.Printf("packetTypes: %v", packetTypes)
}

func onPublicEndpointsRequest(conn *net.UDPConn, address *net.UDPAddr, peerTag PacketSlice) {
	log.Printf("onPublicEndpointsRequest - recv from %v, peer_tag: %v, query_id: %v", address.IP, peerTag)
}

func onUdpPing(conn *net.UDPConn, address *net.UDPAddr, peerTag PacketSlice, queryId PacketSlice) {

	log.Printf("onUdpPing - recv from %v, peer_tag: %v, query_id: %v", address.IP, peerTag, queryId)

	data := make(PacketSlice, 1024)
	offset := 0

	data.Copy(peerTag, 0)
	offset += len(peerTag)
	data.Copy(PacketTypeUint32Max, offset)
	offset += len(PacketTypeUint32Max)
	data.Copy(PacketTypeUint32Max, offset)
	offset += len(PacketTypeUint32Max)
	data.Copy(PacketTypeUint32Max, offset)
	offset += len(PacketTypeUint32Max)
	data.Copy(NewWithUint32(TLID_UDP_REFLECTOR_SELF_INFO), offset)
	offset += len(PacketTypeUint32Max)

	// set time
	time := uint32(time.Now().Unix())
	timeData := make(PacketSlice, 4)
	binary.LittleEndian.PutUint32(timeData, time)
	log.Printf(">>>>> timeData: %v, time: %v", timeData, time)
	data.Copy(timeData, offset)
	offset += len(timeData)

	// set request id
	data.Copy(queryId, offset)
	log.Printf(">>>>> queryId: %v", queryId)
	offset += len(queryId)

	// set addr: 16 bytes
	//addr :=
	addr := make(PacketSlice, 16).Copy(PacketSlice(address.IP.String()), 0)
	log.Printf(">>>>> address.IP: %v, addr: %v", address.IP, addr)
	data.Copy(addr, offset)
	offset += len(addr)

	// set port: 4 bytes
	port := uint32(address.Port)
	portData := make(PacketSlice, 4)
	binary.LittleEndian.PutUint32(portData, port)
	log.Printf(">>>>> portData: %v", portData)
	data.Copy(portData, offset)
	offset += len(portData)

	log.Printf("*********** onUdpPing - send data len: %v", len(data[:offset]))
	// write back
	conn.WriteToUDP(data[:offset], address)
}

type Endpoint struct {
	Addr           *net.UDPAddr
	LastRecvedTime time.Time
}

type RelayTable struct {
	PeerTag string
	Peers   []Endpoint
}

var relayTable = map[string]*RelayTable{}

func onRelayDataAvailable(conn *net.UDPConn, address *net.UDPAddr, peerTag PacketSlice, buf PacketSlice) {

	log.Printf("onRelayDataAvailable - recv from %v, peer_tag: %v, len: %v", address.IP, peerTag, len(buf))

	// std::shared_ptr<RelayTable> table;
	table, ok := relayTable[peerTag.String()]
	if !ok {
		table = &RelayTable{
			PeerTag: peerTag.String(),
			Peers: []Endpoint{{
				Addr:           address,
				LastRecvedTime: time.Now(),
			}},
		}
		relayTable[peerTag.String()] = table
	} else {
		found := false
		for _, peer := range table.Peers {
			if peer.Addr.IP.String() == address.IP.String() && peer.Addr.Port == address.Port {
				found = true
			}
		}
		if !found {
			table.Peers = append(table.Peers, Endpoint{
				Addr:           address,
				LastRecvedTime: time.Now(),
			})
		}
	}

	for _, peer := range table.Peers {
		if peer.Addr.IP.String() != address.IP.String() || peer.Addr.Port != address.Port {
			log.Printf("************** onRelayData - %v", table.Peers)
			log.Printf("************** onRelayData - %v --> %v", address, peer.Addr)
			log.Printf("************** onRelayData - send data len: %v", len(buf))
			conn.WriteToUDP(buf, peer.Addr)
		}
	}
}
