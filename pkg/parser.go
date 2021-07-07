package optimizedconn

import (
	"encoding/binary"
	"fmt"
	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/common"
	"github.com/scionproto/scion/go/lib/snet"
	"net"
)

type PacketSerializer struct {
	baseBytes snet.Bytes
	headerBytes int
	basePayloadBytes int

	listenAddr *net.UDPAddr
	remoteAddr *snet.UDPAddr
}

func NewPacketSerializer(localIA addr.IA, listenAddr *net.UDPAddr, remoteAddr *snet.UDPAddr) (*PacketSerializer, error) {

	scionDestinationAddress := snet.SCIONAddress{
		IA: remoteAddr.IA,
		Host: addr.HostFromIP(remoteAddr.Host.IP),
	}

	scionListenAddress := snet.SCIONAddress{
		IA: localIA,
		Host: addr.HostFromIP(listenAddr.IP),
	}

	var bytes snet.Bytes
	bytes.Prepare()

	preparedPacket := &snet.Packet{
		Bytes: bytes,
		PacketInfo: snet.PacketInfo{
			Destination: scionDestinationAddress,
			Source: scionListenAddress,
			Path: remoteAddr.Path,
			// This is a hack.
			Payload: snet.UDPPayload{
				Payload: make([]byte, 0),
				SrcPort: 0,
				DstPort: 0,
			},
		},
	}

	err := preparedPacket.Serialize()
	if err != nil {
		return nil, err
	}

	headerBytes := len(preparedPacket.Bytes) - 8
	// We use the Packet to calculate the correct sum and subtract our dummy payload.
	basePayloadBytes := int(binary.BigEndian.Uint16(preparedPacket.Bytes[6:8]) - 8)

	fmt.Printf("headerBytes=%v basePayloadBytes=%v\n", headerBytes, basePayloadBytes)

	pS := PacketSerializer{
		listenAddr: listenAddr,
		remoteAddr: remoteAddr,
		baseBytes: preparedPacket.Bytes,
		headerBytes: headerBytes,
		basePayloadBytes: basePayloadBytes,
	}

	return &pS, nil
}

func (pS *PacketSerializer) Serialize(b []byte) ([]byte, error) {

	l4PayloadSize := 8 + len(b)

	// Network Byte Order is Big Endian
	binary.BigEndian.PutUint16(pS.baseBytes[6:8], uint16(pS.basePayloadBytes + l4PayloadSize))

	binary.BigEndian.PutUint16(pS.baseBytes[pS.headerBytes + 0: pS.headerBytes + 2], uint16(pS.listenAddr.Port))
	binary.BigEndian.PutUint16(pS.baseBytes[pS.headerBytes + 2: pS.headerBytes + 4], uint16(pS.remoteAddr.Host.Port))
	binary.BigEndian.PutUint16(pS.baseBytes[pS.headerBytes + 4: pS.headerBytes + 6], uint16(l4PayloadSize))
	binary.BigEndian.PutUint16(pS.baseBytes[pS.headerBytes + 6: pS.headerBytes + 8], uint16(0))

	copy(pS.baseBytes[pS.headerBytes + 8: pS.headerBytes + l4PayloadSize], b)

	dataLength := pS.headerBytes + l4PayloadSize
	return pS.baseBytes[0:dataLength], nil
}

type PacketParser struct {
	readBuffer []byte
}

func NewPacketParser() (*PacketParser, error) {

	readBuffer := make([]byte, common.MaxMTU)

	packetParser := PacketParser{
		readBuffer: readBuffer,
	}

	return &packetParser, nil
}

func (pP *PacketParser) Parse(n int, readBytes []byte) int {
	// Payload is L4 UDP, we need to unpack this too. This has a fixed length of 8 bytes.
	udpPayloadLen := int(binary.BigEndian.Uint16(pP.readBuffer[6:8]))

	payloadLen := udpPayloadLen - 8
	startPos := n - payloadLen

	copy(readBytes, pP.readBuffer[startPos:n])

	return payloadLen
}